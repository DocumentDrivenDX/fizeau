# Matrix Cost Guards

> **Historical note (2026-05-16)**: This procedure ran against the
> retired `fiz-bench matrix` execution path and its `--budget-usd` /
> `--per-run-budget-usd` flags. ADR-016 retired the budget-cap machinery
> entirely; the simplified `./benchmark` runner is not responsible for
> spend caps. The procedure is preserved below as the audit trail for
> how SD-010's matrix cost caps were originally derived. Operators do
> not run these commands anymore.

This directory records the Step 5 procedure that originally derived SD-010
matrix cost caps from observation rather than from a formula guessed before
a run.

## Procedure

1. Use the current anchor profile and the canary task expected to burn the most
   tokens:

   ```sh
   fiz-bench matrix \
     --subset=scripts/beadbench/external/termbench-subset-canary.json \
     --harnesses=fiz \
     --profiles=gpt-5-mini \
     --reps=3 \
     --out=bench/results/cost-observation-$(date -u +%Y%m%dT%H%M%SZ)
   ```

2. Keep only the `git-leak-recovery` cell reports. For each report, record all
   four token streams:

   - `input_tokens`
   - `output_tokens`
   - `cached_input_tokens`
   - `retried_input_tokens`

3. Reconcile cost from the profile pricing source:

   ```text
   cost_usd =
     input_tokens * input_usd_per_mtok / 1_000_000 +
     output_tokens * output_usd_per_mtok / 1_000_000 +
     cached_input_tokens * cached_input_usd_per_mtok / 1_000_000
   ```

   `retried_input_tokens` is tracked as an integrity signal. It is not billed
   separately unless the profile explicitly adds a future retried-input price.

4. Derive caps:

   ```text
   per_run_cap_usd = clamp(p95(observed_cost_usd) * 2.0, 1.00, 5.00)
   per_matrix_cap_usd = per_run_cap_usd * n_runs * 1.2
   ```

5. Run the real matrix with both caps:

   ```sh
   fiz-bench matrix \
     --subset=scripts/beadbench/external/termbench-subset-canary.json \
     --harnesses=fiz,pi,opencode \
     --profiles=gpt-5-mini \
     --reps=3 \
     --per-run-budget-usd=<derived per-run cap> \
     --budget-usd=<derived matrix cap> \
     --out=bench/results/matrix-$(date -u +%Y%m%dT%H%M%SZ)

   fiz-bench matrix-aggregate bench/results/matrix-...
   ```

   `costs.json` records the caps and every cell's token/cost totals.

## Current Verification

The repository-level automated verification uses the no-API `cost_probe`
adapter with the `smoke` profile. It emits deterministic token streams and a
positive synthetic cost. The matrix runner marks each over-budget run as
`process_outcome=budget_halted`, persists `final_status=budget_halted`, and
continues to later tuples.

Live observation requires an API key and operator budget. Do not fabricate live
cost observations; commit the resulting matrix output directory path and cap
derivation only after the paid observation run has completed.
