# SD-013: Provider Signal Matrix for OpenRouter and OpenAI-Compatible Surfaces

This note inventories the routing-evidence surfaces that FEAT-004 and SD-005
already imply, with emphasis on OpenRouter and generic OpenAI-compatible
providers. The point is to separate:

- model inventory and context-limit discovery
- quota and cost evidence
- live utilization evidence

OpenRouter and the generic `openai` wrapper are intentionally not treated as
provider-owned live utilization sources in this step.

## Matrix

| Provider class | Context-limit discovery | Quota headers | Cost attribution | Utilization status |
|----------------|-------------------------|---------------|------------------|--------------------|
| `openrouter` | `internal/provider/openrouter.LookupModelLimits()` reads `GET /models` and maps `context_length` plus `top_provider.max_completion_tokens` into `limits.ModelLimits`. | `internal/provider/quotaheaders.ParseOpenRouter()` parses `x-ratelimit-limit`, `x-ratelimit-remaining`, `x-ratelimit-reset`, and `retry-after`. | `internal/provider/openrouter.UsageCostAttribution()` preserves gateway-reported `usage.cost` as billed USD cost. | `ranking-only`: model inventory, quota, and cost are available, but no provider-owned live utilization probe exists here. Marked unavailable for live utilization; tracked by bead `fizeau-4d01efdc`. |
| `openai` / generic OpenAI-compatible | No provider-owned limit probe in this layer. The shared OpenAI-compatible `/v1/models` discovery path is for model inventory and ranking, not live utilization. | `internal/provider/quotaheaders.ParseOpenAI()` is the default parser installed by `internal/provider/openai.New()` when a quota observer is wired. | Cost attribution is optional and provider-specific; the generic OpenAI-compatible wrapper defaults to `CostSourceUnknown` unless a concrete provider supplies a hook. | `documentation-only`: the shared protocol shape is transport plumbing, not a utilization surface. Marked unavailable for live utilization; tracked by bead `fizeau-4d01efdc`. |

## Existing Probe Map

The matrix above relies on existing code paths rather than new probes:

- `internal/provider/openrouter/openrouter.go`
  - model limit discovery: `LookupModelLimits()`
  - quota parsing: `QuotaHeaderParser: quotaheaders.ParseOpenRouter`
  - cost attribution: `UsageCostAttribution()`
- `internal/provider/openai/openai.go`
  - generic quota parsing: default `quotaheaders.ParseOpenAI`
  - generic cost handling: optional `UsageCostAttribution` hook, otherwise unknown
- `internal/provider/openai/discovery.go`
  - shared OpenAI-compatible model discovery and ranking helpers
- `internal/provider/quotaheaders/quotaheaders.go`
  - OpenAI and OpenRouter rate-limit header parsers

For contrast, the local provider families documented in FEAT-004/SD-005 already
have live utilization probes where applicable:

- `vllm` -> `/metrics`
- `llama-server` -> `/metrics`, then `/slots`
- `omlx` -> `/api/status`
- `rapid-mlx` -> documented status surface

OpenRouter and generic OpenAI-compatible surfaces do not join that group in the
current design.

## Follow-Up Status

- OpenRouter live utilization probe: unavailable in this step; tracked by bead `fizeau-4d01efdc`.
- Generic OpenAI-compatible live utilization probe: unavailable in this step; tracked by bead `fizeau-4d01efdc`.

If a later bead wants live utilization for either class, it needs a provider-
owned probe design rather than reuse of the shared OpenAI-compatible transport
layer.
