---
ddx:
  id: skill-progressive-disclosure-2026-05-01
  created: 2026-05-01
  reviewed_by: pending
status: DRAFT v1
exit_criterion: Filing of epic bead. Subsequent revisions live in child beads.
---

# SKILL.md Progressive Disclosure System

Inspired by the Flue framework's skill-loading pattern.

## Overview

At session start the agent scans a configurable skills directory, reads **only YAML frontmatter** from each `SKILL.md`, and builds a compact catalog. The catalog is injected into the system prompt (~20 tokens overhead + ~15 tokens/skill). A `load_skill` tool returns the full body on demand. Skills are opt-in: the feature activates only when the skills directory exists or is explicitly configured.

## Frontmatter Schema

```yaml
---
name: fix-tests        # required; slug [a-z0-9-_]+, max 64 chars
description: Fix failing tests in a Go project. Use when tests are failing after a code change or when asked to make tests pass.   # required; under 1024 chars
tags: [testing, go]   # optional
version: "1.0"        # optional
---
# Full instructions body...
```

Missing `name` or `description` → skill is skipped with a warning. Unknown YAML fields are silently ignored.

## New Package Layout

```
internal/skill/
  skill.go           # Skill, Frontmatter, Catalog types; ScanDir()
  frontmatter.go     # ParseFrontmatter(); reads only the --- block
  frontmatter_test.go
  catalog_test.go
```

Imports: stdlib + `gopkg.in/yaml.v3` + `internal/safefs`. No imports of `internal/tool` or `internal/prompt` (avoids cycles).

```
internal/skill/skill_tool.go  # LoadSkillTool — lives in internal/skill (not internal/tool)
```

> **Codex review fix:** Putting `LoadSkillTool` in `internal/tool` would require `internal/tool` to import `internal/skill`, creating a cycle since `internal/skill` may use `internal/safefs`. `LoadSkillTool` lives in `internal/skill` instead. The wiring in `agentcli/run.go` assembles the tool directly from `*skill.Catalog` with no intermediary `internal/tool` import.

## System Prompt Injection Format

Added as a `# Available Skills` section via `Builder.WithSkillCatalog(*skill.Catalog)`:

```
# Available Skills

- fix-tests: Fix failing tests in a Go project. Use when tests are failing after a code change.
- scaffold-service: Scaffold a new Go microservice. Use when creating a new service from scratch.

To use a skill, call the `load_skill` tool with the skill name. Always load the skill before beginning the task it describes.
```

Nil or empty catalog → section is omitted entirely. Section entries sorted by name (deterministic output).

## `load_skill` Tool

**Name:** `load_skill`  
**Description:** Load the full instructions for a named skill. Returns the complete SKILL.md body as markdown.  
**Parameters:** `{ "skill_name": string }`  
**On success:** raw markdown body (everything after closing `---`).  
**On unknown name:** `"load_skill: skill \"foo\" not found; available: fix-tests, scaffold-service"`.

## Config Keys

```go
// internal/config/config.go
type SkillsConfig struct {
    Dir string `yaml:"dir,omitempty"` // default: ".fizeau/skills"; "-" = disabled
}

// Added to Config struct:
Skills SkillsConfig `yaml:"skills,omitempty"`
```

> **Codex review fix:** The spec must explicitly add `Skills SkillsConfig` to `config.Config`, and SKILL-5's ACs must include a `TestLoad_SkillsConfig_ParsedFromYAML` test covering round-trip YAML and the `FIZEAU_SKILLS_DIR` env override — matching the pattern of `TestLoad_BashOutputFilterParsedFromYAML`.

Env override: `FIZEAU_SKILLS_DIR`.

Default behavior: if `Skills.Dir` is empty, check for `.fizeau/skills` relative to `workDir`. Absent directory → silently disabled (no error). `"-"` → disabled even if directory exists.

## Wiring (`agentcli/run.go`)

After `buildToolsForPreset`, before `sysPrompt.Build()`:

```go
skillsDir := resolveSkillsDir(cfg, wd)  // env → config → default
skillCatalog, err := skill.ScanDir(skillsDir)  // empty catalog if dir absent/disabled
if skillCatalog.Len() > 0 {
    tools = append(tools, &tool.LoadSkillTool{Catalog: skillCatalog})
}
sysPrompt := prompt.NewFromPreset(preset).
    // ... existing chain ...
    WithSkillCatalog(skillCatalog)
```

`LoadSkillTool` is assembled at the call site (not inside `BuiltinToolsForPreset`) to keep `internal/tool` free of `internal/skill` import.

## Test Strategy

**`internal/skill/frontmatter_test.go`:**
- Valid frontmatter parses correctly, body offset recorded.
- Missing name/description returns validation error.
- No frontmatter returns `ErrNoFrontmatter`.
- Unknown fields silently ignored.

**`internal/skill/catalog_test.go`:**
- `ScanDir` on empty/non-existent dir returns empty catalog + nil error.
- Mix of valid and invalid skills → only valid in catalog.
- `LoadBody` lazy-loads correct content.
- `ByName` lookup works; unknown returns nil.

**`internal/tool/skill_tool_test.go`:**
- Known skill returns body.
- Unknown name returns error listing available skills.
- Empty catalog returns clear message.

**`internal/prompt/builder_test.go` additions:**
- Non-empty catalog → `# Available Skills` section present in `Build()`.
- Nil/empty catalog → section absent.
- Output is deterministic (sorted by name).

**Integration test:** Virtual provider session with `.fizeau/skills/example/SKILL.md` present; verify catalog appears in system prompt and `load_skill` returns correct body.

## Bead Breakdown

| Bead | Title | Deps | Size |
|---|---|---|---|
| SKILL-1 (epic) | SKILL.md progressive disclosure system | — | L |
| SKILL-2 | `internal/skill`: frontmatter parsing + `ScanDir` + `Catalog` | — | M |
| SKILL-3 | `LoadSkillTool` + `run.go` wiring | SKILL-2 | M |
| SKILL-4 | `Builder.WithSkillCatalog` prompt injection | SKILL-2 | S |
| SKILL-5 | `SkillsConfig` + `FIZEAU_SKILLS_DIR` + integration test | SKILL-2, SKILL-3, SKILL-4 | M |

Beads SKILL-3 and SKILL-4 can execute in parallel after SKILL-2.

## Estimated Effort

~1.5–2 engineer-days sequential; ~1 day with SKILL-3/SKILL-4 parallelized.
