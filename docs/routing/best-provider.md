# Candidate Selection

Automatic routing builds a complete candidate list before choosing.

1. Enumerate configured harnesses, provider sources, endpoints, and discovered
   model IDs.
2. Join each discovered model to the catalog for power, context, cost,
   capabilities, provider/deployment class, status, and provenance.
   Profile and target references with ordered catalog candidates are matched
   against live provider discovery in order, so an endpoint can serve a later
   catalog candidate when the primary candidate is absent.
   Provider-native names with unambiguous casing, prefix, quantization, or
   packaging differences, such as `Qwen3.6-27B-MLX-8bit`, use the matching
   catalog model's metadata.
3. Attach live usage and availability signals: health, cooldown, observed
   latency, prepaid quota, reset time, and marginal cost.
4. Apply hard pins for model, provider source/endpoint, and harness.
5. Apply `--min-power` / `--max-power` only when the request is not exact-pinned.
6. Filter by context, tools, reasoning support, health, and catalog status.
7. Score survivors by power, effective cost, quota, availability, speed, context,
   and capability.
8. Dispatch the top candidate once.

Local/free candidates are preferred over paid cloud candidates when they satisfy
the requested power, tools, context, health, and hard constraints. This
preference never overrides a power bound, exact model pin, provider
source/endpoint pin, harness pin, or required capability.

Provider/deployment class is part of power assignment. A local or community
copy does not tie a managed cloud frontier model solely because one benchmark
looks similar.

When no power bound or hard pin is supplied, the score still uses the full
inventory and chooses the best lowest-cost viable auto-routable candidate.

Routing failures return evidence:

- requested power bounds and hard pins
- selected candidate
- rejected candidates and filter reasons
- score components
- attempted-route failure class

The agent does not retry another candidate inside the same request.
