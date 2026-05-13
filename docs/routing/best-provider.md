# Candidate Selection

Automatic routing evaluates `route(client_inputs, fiz_models_snapshot)` after
building the snapshot. The snapshot is the only source of routing facts.

1. Enumerate configured harnesses, provider sources, endpoints, and discovered
   model IDs.
2. Join each discovered model to the catalog for power, context, cost,
   capabilities, provider/deployment class, status, and provenance.
   Provider-native names with unambiguous casing, prefix, quantization, or
   packaging differences, such as `Qwen3.6-27B-MLX-8bit`, use the matching
   catalog model's metadata when the mapping is unambiguous.
3. Attach live usage and availability signals: health, cooldown, observed
   latency, prepaid quota, reset time, effective cost, `actual_cash_spend`, and
   local endpoint utilization when the provider type exposes it.
4. Apply hard pins for model, provider source/endpoint, and harness. Pins can
   consider providers that are not included by default or metered-enabled.
5. Apply explicit user constraints such as `air-gapped` / `no_remote`. These
   requirements beat pins.
6. Apply default inclusion and metered opt-in. Unpinned automatic routing
   excludes default-deny providers and excludes pay-per-token providers unless
   provider default inclusion and explicit metered-spend opt-in both allow
   actual metered spend.
7. Apply `--policy`, `--min-power`, and `--max-power` as automatic-routing
   policy and scoring inputs.
8. Filter by context, tools, reasoning support, health, quota, and catalog
   status when those facts make dispatch impossible.
9. Reuse an existing sticky route assignment when the request belongs to a
   known worker sequence and the assigned endpoint is still eligible.
10. For a new sticky sequence, choose the least-loaded equivalent local endpoint
   using provider utilization plus Fizeau in-flight lease counts.
11. Score survivors by power, effective cost, actual_cash_spend, quota,
   availability, speed, context, endpoint utilization pressure, and capability.
12. Dispatch the top candidate once.

Local/free candidates are preferred over paid cloud candidates when they satisfy
the requested power intent, tools, context, health, and hard constraints. The
router prefers the lowest effective cost candidate whose power fit is
sufficient for the selected policy, so a free but materially underpowered model
does not beat an in-band `default` candidate solely because it is free. A
subscription candidate may still score with PAYG-equivalent effective cost even
when `actual_cash_spend=false`. This preference never overrides an exact model
pin, provider source/endpoint pin, harness pin, or required capability.

Provider/deployment class is part of power assignment. A local or community
copy does not tie a managed cloud frontier model solely because one benchmark
looks similar.

When no power bound or hard pin is supplied, the score still uses the full
non-metered or explicitly metered-in inventory and chooses the best-scoring
viable auto-routable candidate from the snapshot.

Local endpoint load balancing is sticky. If four local servers all expose the
same resolved model, new long-running workers spread across the least-loaded
eligible endpoints, but subsequent requests in the same worker sequence stay on
the endpoint already assigned to that sequence. vLLM and llama-server provide
server utilization probes; when those probes are stale or unavailable, Fizeau
falls back to its own in-flight lease counts.

Routing failures return evidence:

- requested power bounds and hard pins
- selected candidate
- rejected candidates and filter reasons
- score components
- attempted-route failure class

The same evidence should be explainable from `fiz models` plus the request's
explicit client inputs, not from chat context.

The agent does not retry another candidate inside the same request.
