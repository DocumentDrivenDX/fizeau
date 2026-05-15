# Benchmark corpus

The internal benchmark corpus is a *curated*, capability-tagged set of
closed beads worth tracking over time. It feeds the beadbench harness
(see `scripts/beadbench/`) and gives us a stable, named pool of tasks
for harness-vs-harness and model-vs-model comparisons.

The curation **is** the value. The corpus is intentionally not
auto-promoted. Promotion is a deliberate human act, gated on the
question "does this bead teach us something the rest of the corpus
does not?".

## File layout

Three files, all under `scripts/beadbench/`:

```
scripts/beadbench/
  corpus.yaml                       # append-only top-level index
  corpus/
    capabilities.yaml               # controlled-vocabulary tags
    <bead-id>.yaml                  # per-bead detail (one per index entry)
```

### `corpus.yaml`

Top-level index. Append-only. Each entry has:

| field          | required | notes |
| -------------- | -------- | ----- |
| `id`           | yes      | bead id (e.g. `agent-39f79181`) |
| `capability`   | one of   | tag from `capabilities.yaml` (kind=capability) |
| `failure_mode` | one of   | tag from `capabilities.yaml` (kind=failure_mode) |
| `promoted`     | yes      | `YYYY-MM-DD` |
| `promoted_by`  | yes      | identity recording the promotion |

Exactly one of `capability` / `failure_mode` must be set.

### `corpus/<bead-id>.yaml`

Per-bead detail. One file per index entry; the file's `bead_id` must
match an entry in `corpus.yaml`. Required fields: `bead_id`,
`project_root`, `base_rev` (pre-change), `known_good_rev`
(resolution), `captured`, `difficulty` (`easy|medium|hard`),
`prompt_kind` (`implement-with-spec|port-by-analogy|debug-and-fix|review`),
and the same `capability` or `failure_mode` tag as the index. `notes`
is free text describing what makes the bead instructive.

### `corpus/capabilities.yaml`

Controlled vocabulary. Adding a tag here is the *only* way to use it
in `corpus.yaml`. Each tag declares an `id`, an `area` (cli, provider,
harness, catalog, routing, prompt, …), a `kind`
(`capability` or `failure_mode`), and a description.

## Curation rules

A bead earns a slot in the corpus when **all** of the following hold:

1. The bead is closed and has a reachable resolution rev (either
   `closing_commit_sha` on the bead record, or one we can name with
   confidence — multi-commit slices are allowed if the final commit is
   identifiable).
2. The bead exemplifies a `capability` or `failure_mode` we want to
   regression-track. Routine refactors, doc updates, and trivial fixes
   do not qualify, no matter how clean the diff is.
3. Promotion adds signal that the rest of the corpus does not already
   carry. If we already have a sampler-resolver capability bead, the
   second sampler-resolver bead is *probably* not worth promoting unless
   it covers a meaningfully different surface (e.g. a different
   provider seam).

If the bead is being promoted because the agent *failed* on it,
the entry uses `failure_mode` (not `capability`). Failure-mode entries
are valuable: they say "this is the surface the agent has historically
tripped on; treat regressions on this kind of task as model-quality
evidence".

## Promoting a bead

The `fiz corpus promote` subcommand handles the gate, the writes,
and the post-write validation:

```
fiz corpus promote agent-39f79181 \
  --capability sampling-resolver-cli-wiring \
  --difficulty medium \
  --prompt-kind implement-with-spec \
  --notes "Catches CLI-side resolver wiring + telemetry plumbing." \
  --yes
```

What the command does:

- Refuses if the bead is already in `corpus.yaml`.
- Refuses if the bead is not closed in `.ddx/beads.jsonl`.
- Defaults `base_rev` to the bead's `parent_commit_sha` and
  `known_good_rev` to its `closing_commit_sha`. Override with
  `--base-rev` / `--known-good-rev` for multi-commit slices.
- Defaults `project_root` to the workdir and `promoted_by` to `$USER`.
- Without `--yes`, shows the plan and asks for confirmation.
- Writes both files atomically (temp + rename). Validates after write;
  if validation fails, both writes are rolled back and the command
  returns the validation error.

`fiz corpus validate` re-runs the cross-validator at any time.
`fiz corpus list` shows the index in a tabular format
(`--json` for machine output).

## Filtering beadbench by corpus

`scripts/beadbench/run_beadbench.py` accepts two corpus-aware flags:

- `--corpus-only` — restrict the task set to beads whose `bead_id`
  appears in `corpus.yaml`. Useful for reproducible apples-to-apples
  arm comparisons.
- `--capability=<tag>` — restrict to corpus beads tagged with the
  given capability (or failure_mode). Useful for "how do these arms do
  on this surface specifically?" investigations.

Both filters are pure intersections: a task must already exist in
`manifest-v1.json` for it to be considered. A capability tag with no
matching task in the manifest produces an empty selection.

## Why curated, not automatic?

The corpus is an editorial artifact. Auto-promotion would either:

- Promote everything (adds noise, dilutes signal — most beads are not
  instructive enough to be worth a regression test), or
- Promote on some heuristic (commit size, test coverage delta, etc.)
  that is easy to game and hard to interpret.

Curation forces us to articulate, in the bead's `notes` field, *why*
this bead is worth tracking. That articulation is the corpus's
durable value: six months from now we want to know what each bead
is testing, not just that it once passed.
