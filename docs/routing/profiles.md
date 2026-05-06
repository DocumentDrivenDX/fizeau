# Legacy Routing Names

This page is retained for old links only.

The target routing interface is numeric power plus hard constraints. Use
`--min-power`, `--max-power`, `--model`, `--provider`, `--harness`, and
`fiz --list-models`.

Old named routing macros are compatibility data, not the target interface.
They must not be used for new automation.

When automatic routing selects among equivalent local endpoints, the selected
endpoint is sticky for the request sequence. New sequences are assigned by
least-loaded endpoint; repeated requests with the same sticky key keep using
their assigned endpoint unless it becomes unavailable or hard-saturated.
