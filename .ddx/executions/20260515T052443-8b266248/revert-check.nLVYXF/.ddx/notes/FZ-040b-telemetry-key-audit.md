# FZ-040b Telemetry Key Audit — Custom Product Telemetry Keys

Scope: audit of all custom product-owned OTel attribute keys and slog log-field
keys for old-product-name encoding after the rename from `agent` to `fizeau`.

## Audit Outcome

**No custom keys encode the old product name.** The `ddx.*` namespace is the
DDX platform prefix (unchanged by the Fizeau rename). No key string contains
`agent` as a product-name component. Values emitted at runtime were corrected in
a prior bead (commit `2cf2126`): `ddx.harness.name` now emits `"fizeau"` via
`telemetry.ServiceName`; `gen_ai.agent.name` emits `"Fizeau"` via
`telemetry.ProductName`.

## Custom DDX Extension Keys (`ddx.*`)

All keys are defined as constants in `telemetry/telemetry.go`. None encodes the
old product name.

| Constant | Key string | Notes |
|---|---|---|
| `KeyHarnessName` | `ddx.harness.name` | Value: `"fizeau"` (via `telemetry.ServiceName`) |
| `KeyHarnessVersion` | `ddx.harness.version` | Value: build-time version string |
| `KeySessionID` | `ddx.session.id` | |
| `KeyParentSessionID` | `ddx.parent.session.id` | |
| `KeyRequestedModelRef` | `ddx.request.model_ref` | |
| `KeyTurnIndex` | `ddx.turn.index` | |
| `KeyAttemptIndex` | `ddx.attempt.index` | |
| `KeyToolExecutionIndex` | `ddx.tool.execution.index` | |
| `KeyProviderSystem` | `ddx.provider.system` | |
| `KeyProviderRoute` | `ddx.provider.route` | |
| `KeyAttemptedProviders` | `ddx.routing.attempted_providers` | |
| `KeyFailoverCount` | `ddx.routing.failover_count` | |
| `KeyProviderModelResolved` | `ddx.provider.model_resolved` | |
| `KeyCostSource` | `ddx.cost.source` | |
| `KeyCostCurrency` | `ddx.cost.currency` | |
| `KeyCostAmount` | `ddx.cost.amount` | |
| `KeyCostInputAmount` | `ddx.cost.input_amount` | |
| `KeyCostOutputAmount` | `ddx.cost.output_amount` | |
| `KeyCostCacheReadAmount` | `ddx.cost.cache_read_amount` | |
| `KeyCostCacheWriteAmount` | `ddx.cost.cache_write_amount` | |
| `KeyCostReasoningAmount` | `ddx.cost.reasoning_amount` | |
| `KeyCostPricingRef` | `ddx.cost.pricing_ref` | |
| `KeyCostRaw` | `ddx.cost.raw` | |
| `KeyTimingFirstTokenMS` | `ddx.timing.first_token_ms` | |
| `KeyTimingQueueMS` | `ddx.timing.queue_ms` | |
| `KeyTimingPrefillMS` | `ddx.timing.prefill_ms` | |
| `KeyTimingGenerationMS` | `ddx.timing.generation_ms` | |
| `KeyTimingCacheReadMS` | `ddx.timing.cache_read_ms` | |
| `KeyTimingCacheWriteMS` | `ddx.timing.cache_write_ms` | |

## Allowlisted — External Standard Keys

These keys are defined by external semantic-convention specifications and cannot
be renamed. They are locked by `TestGenAIAgentKeysRemainStandard` in
`telemetry/telemetry_test.go`.

| Constant | Key string | Standard |
|---|---|---|
| `KeyServiceName` | `service.name` | OTel Resource semantic conventions |
| `KeyConversationID` | `gen_ai.conversation.id` | OTel GenAI semantic conventions |
| `KeyAgentName` | `gen_ai.agent.name` | OTel GenAI — "agent" is the spec term for AI agent, not the product name |
| `KeyAgentVersion` | `gen_ai.agent.version` | OTel GenAI semantic conventions |
| `KeyAgentID` | `gen_ai.agent.id` | OTel GenAI semantic conventions |
| `KeyOperationName` | `gen_ai.operation.name` | OTel GenAI semantic conventions |
| `KeyProviderName` | `gen_ai.provider.name` | OTel GenAI semantic conventions |
| `KeyRequestModel` | `gen_ai.request.model` | OTel GenAI semantic conventions |
| `KeyResponseModel` | `gen_ai.response.model` | OTel GenAI semantic conventions |
| `KeyToolName` | `gen_ai.tool.name` | OTel GenAI semantic conventions |
| `KeyToolType` | `gen_ai.tool.type` | OTel GenAI semantic conventions |
| `KeyToolCallID` | `gen_ai.tool.call.id` | OTel GenAI semantic conventions |
| `KeyUsageInput` | `gen_ai.usage.input_tokens` | OTel GenAI semantic conventions |
| `KeyUsageOutput` | `gen_ai.usage.output_tokens` | OTel GenAI semantic conventions |
| `KeyUsageCacheRead` | `gen_ai.usage.cache_read.input_tokens` | OTel GenAI semantic conventions |
| `KeyUsageCacheWrite` | `gen_ai.usage.cache_creation.input_tokens` | OTel GenAI semantic conventions |
| `KeyTokenType` | `gen_ai.token.type` | OTel GenAI semantic conventions |
| `KeyServerAddress` | `server.address` | OTel network semantic conventions |
| `KeyServerPort` | `server.port` | OTel network semantic conventions |
| `KeyErrorType` | `error.type` | OTel semantic conventions |

## Log Fields (slog)

`grep -r 'slog\.String\|slog\.Int\|slog\.Bool\|slog\.Any\|slog\.Attr' --include='*.go'`
returned no field keys containing the old product name. No log field keys require
renaming.

## Verification

```
go test ./telemetry/... ./internal/core/...
```

Both packages pass. `TestGenAIAgentKeysRemainStandard` asserts that
`gen_ai.agent.*` keys are unchanged (allowlist guard).
`TestRun_CONTRACT001SpanConformance` asserts `ddx.harness.name="fizeau"` and
`gen_ai.agent.name="Fizeau"` at runtime.
