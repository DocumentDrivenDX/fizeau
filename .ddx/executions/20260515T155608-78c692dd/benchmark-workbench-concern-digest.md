# Benchmark Workbench Concern Digest

Date: 2026-05-15
Bead: `fizeau-96e8e4d6`
Spec: `FEAT-008`

## Open bead scan

`jq -c 'select(.status=="open") | select((.labels // []) | any(.=="area:ui" or .=="area:data" or .=="area:website" or .=="area:benchmark")) | {id,title,labels,spec:(."spec-id" // null),updated_at,notes}' .ddx/beads.jsonl`

Result at execution time:

- `fizeau-96e8e4d6` — `benchmark workbench: declare microsite and data-pipeline concerns`

No additional open benchmark UI/data beads were present in this execution
worktree's tracker snapshot, so this digest covers the only open benchmark
UI/data bead.

## Concern decision

- Activate `hugo-hextra` for `area:ui`.
- Activate `python-uv` for `area:data`.

## Project-specific overrides

- Keep Hugo + Hextra as the benchmark workbench's docs shell under
  `website/`, with the workbench page rooted at
  `website/content/benchmarks/explorer/_index.md`.
- Allow custom analytical UI code under `website/assets/` for the
  workbench's dense controls, Perspective grid wiring, and pairwise tables,
  while keeping `website/DESIGN.md` as the governing visual constraint.
- Treat `scripts/website/build-benchmark-data.py` plus
  `scripts/website/requirements.txt` as the active Python pipeline instead of
  a repo-wide `pyproject.toml` + `uv sync` application layout.
- Treat `scripts/website/test_build_benchmark_data.py` as the checked-in
  builder regression surface.

## Expected local quality gates

- `npm run build:benchmark-workbench`
- `hugo --source website`
- `make benchmark-workbench-smoke`
- `python3 -m unittest scripts.website.test_build_benchmark_data`
- `python3 -m compileall -q scripts/website/build-benchmark-data.py`
- `uv run --with PyYAML --with pyarrow --with duckdb python scripts/website/build-benchmark-data.py`
  or `make benchmark-data`
