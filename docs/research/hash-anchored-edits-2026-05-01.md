---
ddx:
  id: hash-anchored-edits-2026-05-01
  created: 2026-05-01
  reviewed_by: pending
status: DRAFT v1
exit_criterion: Filing of epic bead (ANC-0). Subsequent revisions live in child beads.
---

# Hash-Anchored Edits (Anchor Mode)

Inspired by Dirac's signature token-efficiency technique. Dirac reports 60–65% reduction in edit output tokens.

## Scope: Simplified v1 (no Myers diff refresh)

**Recommendation: (b) simplified version.**

Full Dirac has three hard sub-problems: (1) vetted single-token vocabulary, (2) session state plumbing, (3) Myers diff refresh to propagate anchors across successive edits. The primary value of anchors is the first read → first edit round; most agent edits touch a file once or twice per session. Anchors that go stale after first write are still better than the status quo.

**v1 includes:**
- AnchorStore: thread-safe per-session state, file → (line → anchor word).
- Modified `ReadTool` (anchor mode): prefixes each output line with `Word: `.
- New `AnchorEditTool` (`anchor_edit`): takes path + start/end anchors + new_text; resolves to line numbers; splices; invalidates anchors for that path.
- Opt-in: `--anchors` flag + `anchors: true` config key.

**v1 excludes:**
- Myers diff anchor refresh (deferred to v2: ~8 additional story points).
- Automatic invalidation on `write`/`edit`/`patch` tools.
- Anchor state persistence across `Run` calls.

**Stale-state mitigation (Codex review):** Since `write`/`edit`/`patch` remain available in anchor mode and do not call `Invalidate`, two guards are required: (a) `anchor_edit` must verify the file's current line count matches the stored anchor count before splicing — mismatch returns "file changed since anchors assigned; re-read"; (b) ANC-6 system prompt addendum must explicitly say: "Do not mix `edit`/`write` with `anchor_edit` on the same file. Re-read to get fresh anchors after any non-anchor change."

## Anchor Word Generation

**~1,024 single-token English nouns, capital-first, committed as a static file.**

`internal/tool/anchorwords/words.go` — a `var Anchors = [1024]string{...}`.

Generation process (one-time offline script, not part of build):
1. Candidate pool: 6–12 char common English nouns, capital-first form.
2. Filter: remove `'`, `-`, digits, non-ASCII, Go keywords, common stopwords.
3. Tokenizer check: retain only words mapping to exactly 1 token in tiktoken cl100k_base AND a Llama BPE tokenizer.
4. Trim to 1,024.

**Runtime assignment:** Line index `i` (0-based) → `Anchors[i % 1024]`.

**Large-file ambiguity:** Files > 1,024 lines have duplicate anchor words. `AnchorEditTool` returns an error when anchors are ambiguous without an `offset_hint` param.

## Session State Design

**New package: `internal/tool/anchorstore`**

```go
type AnchorStore struct {
    mu    sync.RWMutex
    state map[string]*fileAnchors  // key = resolved absolute path
}

// fileAnchors
//   lines []string  // anchor word per line (0-based)
//   valid bool      // false after any invalidation

func New() *AnchorStore

// Assign records anchors for a just-read file slice. fileOffset is the
// 0-based line number of the first element in lines (matches ReadTool's
// offset param so absolute line numbers are correct).
func (s *AnchorStore) Assign(path string, fileOffset int, lines []string)

// Lookup returns the unique 0-based line index for anchor in path.
// Returns (line, false) on unique match, (-1, true) if ambiguous (file >1024
// lines and anchor wraps), (-1, false) if not found or store invalid.
func (s *AnchorStore) Lookup(path, anchor string) (line int, ambiguous bool)

func (s *AnchorStore) Invalidate(path string)
```

> **Codex review fixes:** (1) `Assign` gains `fileOffset int` so absolute line numbers are correct for partial reads. (2) `Lookup` distinguishes not-found vs ambiguous via the `ambiguous bool` return. `AnchorEditTool` uses `offset_hint` to resolve ambiguous cases.

Thread safety: write-lock for `Assign`/`Invalidate`, read-lock for `Lookup`.

Scope: one instance per `Run` call, passed by pointer into tool constructors. NOT part of `core.Request`.

## Modified Read Tool

```go
type ReadTool struct {
    WorkDir     string
    AnchorStore *anchorstore.AnchorStore // nil = legacy mode (unchanged)
}
```

Anchor mode output format:
```
Moderator: func main() {
Ripple:    fmt.Println("hello")
Corona: }
```

- Each line: `"Word: content\n"` (no padding for alignment — token efficiency).
- `AnchorStore.Assign` called with the slice of anchor words for returned lines.
- Truncation marker gets `...` prefix (not a vocabulary word — cannot be used as edit target).
- Legacy mode (nil store): zero change to current output.

## AnchorEditTool

New tool (not a modification of `EditTool`):

**Name:** `anchor_edit`  
**Parameters:**
```json
{
  "path":         string,
  "start_anchor": string,  // anchor word for first line to replace (inclusive)
  "end_anchor":   string,  // anchor word for last line to replace (inclusive)
  "new_text":     string,  // replacement text; empty = delete range
  "offset_hint":  integer  // optional; disambiguates anchor for files > 1024 lines
}
```

**Execute:**
1. Resolve path.
2. Lookup start/end anchors; error if missing or store invalid for file.
3. Handle ambiguity via `offset_hint`; error if ambiguous without hint.
4. Validate `startLine <= endLine`.
5. Splice: `lines[:start] + new_text_lines + lines[end+1:]`.
6. Write back.
7. `AnchorStore.Invalidate(path)`.
8. Return: `"Replaced lines Moderator-Corona (N lines) in path. Anchor map invalidated; re-read to get fresh anchors."`.

`Parallel() bool { return false }`.

## Myers Diff (v2)

Deferred. Would promote `go-difflib` from indirect to direct, add `anchorstore.Reanchor(path, oldLines, newLines)` using `GetOpCodes()` to propagate `equal`-block anchors. ~8 additional story points.

## Test Strategy

**`internal/tool/anchorstore/store_test.go`:**
- Assign + lookup by word returns correct line indices.
- Invalidate clears lookup.
- Concurrent reads: no race (`-race` flag).
- Large file wrapping: line 0 and line 1024 both get "Moderator"; `Lookup` returns 0 (first occurrence).

**`internal/tool/anchorwords/words_test.go`:**
- No duplicates, no empty words, exactly 1,024 entries, no Go keywords.

**`internal/tool/read_test.go` additions:**
- Anchor mode injects prefix.
- Legacy mode (nil store) unchanged.
- Truncation marker gets `...` prefix, not a vocabulary word.
- `offset` parameter carries through to anchor assignment.

**`internal/tool/anchor_edit_test.go`:**
- Basic replace, delete range (empty new_text), stale anchors error, invalidated anchors error, `startLine > endLine` error, ambiguous anchor without hint, ambiguous anchor with hint resolves.

**`agentcli` test:**
- `--anchors` registers `anchor_edit`; without flag, `anchor_edit` absent.

## Bead Breakdown

| Bead | Title | Deps | Size |
|---|---|---|---|
| ANC-0 (epic) | Hash-anchored edits (anchor mode) | — | L |
| ANC-1 | Generate anchor word list; create `anchorwords` package | — | S (2 pts) |
| ANC-2 | `anchorstore` package: thread-safe session state | ANC-1 | S (3 pts) |
| ANC-3 | Modify `ReadTool` to accept optional `AnchorStore`, emit prefixed output | ANC-2 | S (3 pts) |
| ANC-4 | Implement `AnchorEditTool` (`anchor_edit`) | ANC-3 | M (5 pts) |
| ANC-5 | `--anchors` CLI flag + `anchors: true` config + conditional tool registration | ANC-4 | S (3 pts) |
| ANC-6 | Update system prompt when anchor mode active | ANC-5 | S (1 pt) |

**Total: 17 story points.** ANC-4 is the risk item — line-splice off-by-one errors are the primary hazard; acceptance criteria must name all 7 test cases explicitly.

## Note on Precedence

This feature (ANC-*) is higher effort than PIVOT and PRESET changes. Recommend shipping PIVOT + PRESET first (likely 2–3× the pass rate improvement per story point) and treating ANC-* as a follow-on sprint.
