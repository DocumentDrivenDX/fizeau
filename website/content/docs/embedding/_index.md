---
title: Embedding (Go library)
linkTitle: Embedding
weight: 30
---

{{< callout type="info" >}}
This page is a curated guided tour. The authoritative API reference lives at
[pkg.go.dev](https://pkg.go.dev/github.com/easel/fizeau).
{{< /callout >}}

## Overview

Fizeau is a Go library that implements a coding agent runtime. The root
package `github.com/easel/fizeau` is a thin facade: it exports the
`FizeauService` interface, request/response/event types, and the
`New` constructor. All concrete execution, transcript, routing health,
and quota mechanics live behind `internal/` packages.



## Quick start

```go
package main

import (
    "context"
    "fmt"

    "github.com/easel/fizeau"
    _ "github.com/easel/fizeau/configinit"
)

func main() {
    svc, err := fizeau.New(fizeau.ServiceOptions{})
    if err != nil {
        panic(err)
    }
    events, err := svc.Execute(context.Background(), fizeau.ServiceExecuteRequest{
        Prompt:  "Read main.go and tell me the package name",
        Policy:  "cheap",
        WorkDir: ".",
    })
    if err != nil {
        panic(err)
    }
    for event := range events {
        fmt.Println(event.Type)
    }
}
```

The blank import of `github.com/easel/fizeau/configinit` registers the
default YAML config loader via `RegisterConfigLoader`; without it,
`New` starts with no provider config and `ListProviders`/`HealthCheck`
will error until config is injected explicitly.

## Core types


### `FizeauService`

FizeauService is the entire public Go surface of the fizeau module.

```go
type FizeauService interface {
    Execute(ctx context.Context, req ServiceExecuteRequest) (<-chan ServiceEvent, error)
    TailSessionLog(ctx context.Context, sessionID string) (<-chan ServiceEvent, error)
    ListHarnesses(ctx context.Context) ([]HarnessInfo, error)
    ListProviders(ctx context.Context) ([]ProviderInfo, error)
    ListModels(ctx context.Context, filter ModelFilter) ([]ModelInfo, error)
    ListPolicies(ctx context.Context) ([]PolicyInfo, error)
    HealthCheck(ctx context.Context, health HealthTarget) error
    ResolveRoute(ctx context.Context, req RouteRequest) (*RouteDecision, error)
    RecordRouteAttempt(ctx context.Context, attempt RouteAttempt) error
    RouteStatus(ctx context.Context) (*RouteStatusReport, error)

    // Historical session-log projections. CONTRACT-003 owns these so CLI
    // subcommands such as log/replay/usage do not need to read the
    // internal session-log JSONL schema.
    UsageReport(ctx context.Context, opts UsageReportOptions) (*UsageReport, error)
    ListSessionLogs(ctx context.Context) ([]SessionLogEntry, error)
    WriteSessionLog(ctx context.Context, sessionID string, w io.Writer) error
    ReplaySession(ctx context.Context, sessionID string, w io.Writer) error
}
```


### `ServiceOptions`

ServiceOptions configures a FizeauService instance.

```go
type ServiceOptions struct {
    seamOptions

    ConfigPath string    // optional override; default $XDG_CONFIG_HOME/fizeau/config.yaml
    Logger     io.Writer // optional; agent writes structured session logs internally regardless

    // ServiceConfig, when non-nil, supplies provider and routing data for
    // ListProviders and HealthCheck. Pass a value wrapping the loaded config.
    // When nil, those methods return an error.
    ServiceConfig ServiceConfig

    // QuotaRefreshDebounce is the minimum interval between live quota probes for
    // a primary subscription harness. Zero uses the service default.
    QuotaRefreshDebounce time.Duration
    // QuotaRefreshStartupWait bounds startup waiting when the durable quota
    // cache is missing, stale, or incomplete. Zero uses the service default.
    QuotaRefreshStartupWait time.Duration
    // QuotaRefreshInterval enables periodic refresh for long-running server
    // processes. Zero disables the timer; cache refresh still happens on startup
    // and service activity.
    QuotaRefreshInterval time.Duration
    // QuotaRefreshContext optionally cancels the periodic server refresh worker.
    // When nil, the worker uses context.Background().
    QuotaRefreshContext context.Context

    // CatalogProbeTimeout bounds live /v1/models discovery during routing.
    // Zero uses the service default of 2s.
    CatalogProbeTimeout time.Duration
    // CatalogReloadTimeout bounds stale-while-revalidate catalog reloads.
    // Zero uses the service default of 30s.
    CatalogReloadTimeout time.Duration

    // LocalCostUSDPer1kTokens is the operator-supplied electricity/operations
    // estimate for local endpoint providers under the embedded agent harness.
    // Zero means local endpoint cost is unknown.
    LocalCostUSDPer1kTokens float64
    // SubscriptionCostCurve optionally overrides the default subscription
    // effective-cost curve used by routing.
    SubscriptionCostCurve *SubscriptionCostCurve

    // SessionLogDir overrides the directory used by historical session-log
    // projections (UsageReport, ListSessionLogs, WriteSessionLog,
    // ReplaySession). Empty falls back to ServiceConfig.SessionLogDir().
    // Per-Execute requests still set their own
    // ServiceExecuteRequest.SessionLogDir.
    SessionLogDir string

    // StaleHarnessReaperGrace is the minimum age before a startup reaper may
    // terminate an owned subprocess record. Zero uses the default grace window.
    StaleHarnessReaperGrace time.Duration

    // HealthProbeInterval is the interval between background aliveness probes
    // for configured non-cloud providers. Zero uses the default (60s).
    HealthProbeInterval time.Duration
    // HealthSignalTTL is the maximum age of a probe result before it expires
    // for routing purposes. Zero uses the default (10 min).
    HealthSignalTTL time.Duration
    // PersistRouteHealth, when non-empty, is the file path where probe results
    // are persisted across processes so a fresh process skips redundant probing.
    PersistRouteHealth string
    // AlivenessProber, when non-nil, overrides the default TCP-connect prober
    // used during startup and periodic aliveness probing. Nil uses the default.
    AlivenessProber ProviderAlivenessProber
}
```


### `ServiceExecuteRequest`

ServiceExecuteRequest is the public ExecuteRequest type per CONTRACT-003.

```go
type ServiceExecuteRequest struct {
    Prompt            string
    SystemPrompt      string
    Model             string
    Provider          string
    Harness           string
    Policy            string
    WorkDir           string
    Temperature       *float32
    TopP              *float64
    TopK              *int
    MinP              *float64
    RepetitionPenalty *float64
    Seed              *int64
    // SamplingSource is the comma-separated layer attribution produced by
    // internal/sampling.Resolve. Plumbed through to the llm.request
    // telemetry event for ADR-007 override-tracking; never on the wire.
    SamplingSource string
    Reasoning      Reasoning
    NoStream       bool
    Permissions    string
    // Tools overrides the built-in native agent tool set when Harness is
    // "fiz". Nil uses the native built-ins for ToolPreset and WorkDir.
    Tools []Tool
    // ToolPreset is passed through to BuiltinToolsForPreset when Tools is nil.
    // Empty uses the default tool set.
    ToolPreset string

    // PlanningMode, when true, performs one no-tool LLM "planning" call
    // before the main native agent loop and injects the response as an
    // assistant message wrapped in <plan> tags. The benchmark tool preset
    // auto-enables planning; this flag is the per-request opt-in for other
    // callers (e.g. the CLI --plan flag). Only honored when Harness=="fiz"
    // (the native loop). See internal/core.Request.PlanningMode.
    PlanningMode bool

    // EstimatedPromptTokens, when > 0, drives auto-selection's
    // context-window gate (filter out candidates whose context window is
    // too small for the prompt + safety margin).
    EstimatedPromptTokens int
    // RequiresTools, when true, drives auto-selection's tool-support gate
    // (filter out candidates that cannot invoke tools).
    RequiresTools bool
    MinPower      int
    MaxPower      int

    // CachePolicy is the public opt-out for prompt caching. Empty (the zero
    // value) and "default" both request the per-provider default caching
    // behavior; "off" disables caching for this request. Any other value is
    // rejected at the service boundary (see ValidateCachePolicy). Providers
    // that do not implement caching ignore the field; this field is read by
    // the Anthropic cache_control writer (bead C) and the cache-aware cost
    // attribution path (bead D).
    CachePolicy string

    // Three independent timeout knobs:
    //   Timeout         — wall-clock cap on the entire request.
    //   IdleTimeout     — streaming-quiet cap; per-stream gap.
    //   ProviderTimeout — per-HTTP-request cap to the provider.
    Timeout         time.Duration
    IdleTimeout     time.Duration
    ProviderTimeout time.Duration

    // Optional native-loop overrides used when Harness == "fiz". These let
    // the CLI and other callers route fully-resolved execution settings through
    // the service path instead of maintaining a divergent direct loop path.
    MaxIterations           int
    MaxTokens               int
    ReasoningByteLimit      int
    CompactionContextWindow int
    CompactionReserveTokens int

    // CostCapUSD is the per-run cost cap in USD, mirrored to
    // internal/core.Request.CostCapUSD. When > 0, the native loop halts
    // before issuing the next llm.request once running known cost plus the
    // projected next-turn cost would meet or exceed the cap; the resulting
    // final event reports Status="budget_halted". Zero means no cap. Per
    // FEAT-005 §28 / AC-FEAT-005-07, the cap requires turn cost to be known;
    // unknown-cost runs proceed past the cap with a stderr warning. Honored
    // only when Harness=="fiz" (the native loop) — subprocess harnesses
    // manage cost externally.
    CostCapUSD float64

    // Optional stall policy. When non-nil agent enforces and ends with
    // Status="stalled" if any limit hits. Default policy applies when nil.
    StallPolicy *StallPolicy

    // SessionLogDir overrides the default session-log directory for this
    // request (e.g. an execute-bead per-bundle evidence directory).
    SessionLogDir string

    // SelectedRoute is the configured model-route name the caller picked
    // (e.g. "code-pool"). Recorded into the service-owned session log so
    // post-hoc routing analytics can correlate logs to route keys without
    // reconstructing attribution from the event stream.
    SelectedRoute string

    // Metadata is bidirectional: echoed back in every Event AND stamped
    // onto every line of the session log so external consumers correlate.
    Metadata map[string]string

    // Role tags the kind of work this call performs (e.g. "implementer",
    // "reviewer", "decomposer", "summarizer"). Observational: echoed into
    // the routing_decision and final event Metadata, plus the session-log
    // header. Per CONTRACT-003 it is NOT part of the selection precedence
    // chain (Day 1) and does NOT affect routing. Empty means unset.
    //
    // Normalization: lowercased, alphanumeric + hyphen only, max 64 chars;
    // invalid values are rejected pre-dispatch with RoleNormalizationError.
    Role string
    // CorrelationID links calls that share work context (e.g.
    // "bead_123:attempt_4") so reviewer/implementer/retry attempts can be
    // joined in logs and aggregations. Observational: echoed into
    // routing_decision and final event Metadata, plus the session-log
    // header. Per CONTRACT-003 it is NOT part of the selection precedence
    // chain (Day 1) and does NOT affect routing. Empty means unset.
    //
    // Normalization: printable ASCII (no control chars, no whitespace
    // except hyphen, colon, underscore), max 256 chars; invalid values
    // are rejected pre-dispatch with CorrelationIDNormalizationError.
    CorrelationID string
}
```


### `ServiceEvent`

ServiceEvent is a contract-level event (mirrors harnesses.

```go
type ServiceEvent = harnesses.Event
```


### `ServiceConfig`

ServiceConfig provides provider and routing data to the service without.

```go
type ServiceConfig interface {
    // ProviderNames returns provider names in stable order (default first).
    ProviderNames() []string
    // DefaultProviderName returns the name of the configured default provider.
    DefaultProviderName() string
    // Provider returns the raw config values for a named provider.
    Provider(name string) (ServiceProviderEntry, bool)
    // HealthCooldown returns the configured cooldown duration (0 = use default 30s).
    HealthCooldown() time.Duration
    // WorkDir is the base directory for file-backed health state.
    WorkDir() string
    // SessionLogDir returns the configured sessions directory.
    SessionLogDir() string
}
```


### `ModelFilter`

ModelFilter filters ListModels results.

```go
type ModelFilter struct {
    Harness  string
    Provider string
}
```


### `ModelInfo`

ModelInfo describes a model with full metadata per CONTRACT-003.

```go
type ModelInfo struct {
    ID                            string
    Provider                      string
    ProviderType                  string
    Harness                       string
    EndpointName                  string
    EndpointBaseURL               string
    ServerInstance                string
    ContextLength                 int
    ContextSource                 string
    Utilization                   RouteUtilizationState
    Capabilities                  []string
    Cost                          CostInfo
    PerfSignal                    PerfSignal
    Power                         int
    AutoRoutable                  bool
    ExactPinOnly                  bool
    Billing                       BillingModel
    ActualCashSpend               bool
    EffectiveCost                 float64
    EffectiveCostSource           string
    SupportsTools                 bool
    DeploymentClass               string
    HealthFreshnessAt             time.Time
    HealthFreshnessSource         string
    QuotaFreshnessAt              time.Time
    QuotaFreshnessSource          string
    ModelDiscoveryFreshnessAt     time.Time
    ModelDiscoveryFreshnessSource string
    Available                     bool
    IsDefault                     bool // matches the configured default model
    RankPosition                  int  // ordinal in latest discovery rank; -1 if unranked
}
```


### `ProviderInfo`

ProviderInfo describes a provider with live status per CONTRACT-003.

```go
type ProviderInfo struct {
    Name             string
    Type             string // "openai" | "openrouter" | "lmstudio" | "omlx" | "ollama" | "anthropic" | "virtual"
    BaseURL          string
    Endpoints        []ServiceProviderEndpoint
    Status           string // "connected" | "unreachable" | "error: <msg>"
    ModelCount       int
    Capabilities     []string       // e.g. {"tool_use","streaming","json_mode"}
    Billing          BillingModel   // "fixed" | "per_token" | "subscription" | ""
    IncludeByDefault bool           // participates in unpinned/default routing
    IsDefault        bool           // matches the configured default_provider
    DefaultModel     string         // per-provider configured default model, if any
    CooldownState    *CooldownState // nil if not in cooldown
    Auth             AccountStatus
    EndpointStatus   []EndpointStatus
    Quota            *QuotaState
    UsageWindows     []UsageWindow
    LastError        *StatusError
}
```


### `HarnessInfo`

HarnessInfo describes a registered harness as defined in CONTRACT-003.

```go
type HarnessInfo struct {
    Name                 string
    Type                 string // "native" | "subprocess"
    Available            bool
    Path                 string
    Error                string
    Billing              BillingModel
    AutoRoutingEligible  bool
    TestOnly             bool
    ExactPinSupport      bool
    DefaultModel         string   // built-in default model when no override is supplied
    SupportedPermissions []string // subset of {"safe","supervised","unrestricted"}
    SupportedReasoning   []string // values such as {"low","medium","high","xhigh","max"}
    CostClass            string   // "local" | "cheap" | "medium" | "expensive"
    Quota                *QuotaState
    Account              *AccountStatus
    UsageWindows         []UsageWindow
    LastError            *StatusError
    CapabilityMatrix     HarnessCapabilityMatrix
}
```


### `RouteRequest`

RouteRequest specifies a routing query.

```go
type RouteRequest struct {
    Policy      string // optional named policy bundle: cheap|default|smart|air-gapped
    Model       string
    Provider    string
    Harness     string
    Reasoning   Reasoning
    Permissions string
    AllowLocal  bool
    Require     []string
    MinPower    int
    MaxPower    int

    // EstimatedPromptTokens, when > 0, filters out candidates whose context
    // window cannot accommodate the prompt (with a safety margin).
    EstimatedPromptTokens int

    // RequiresTools, when true, filters out candidates that do not support
    // tool calling.
    RequiresTools bool

    // CachePolicy mirrors ServiceExecuteRequest.CachePolicy. Routing decisions
    // today do not act on it; it is carried through so callers using
    // ResolveRoute as a public surface can plumb the same opt-out the
    // Execute path honors.
    CachePolicy string

    // Role tags the kind of work this call performs (e.g. "implementer",
    // "reviewer", "decomposer"). Observational only — it does NOT enter the
    // routing precedence chain. Mirrors ServiceExecuteRequest.Role so
    // ResolveRoute previews don't diverge from Execute.
    Role string

    // CorrelationID joins calls that share work context (e.g.
    // "bead_123:attempt_4"). Observational only — it does NOT enter the
    // routing precedence chain. Mirrors ServiceExecuteRequest.CorrelationID.
    CorrelationID string

    // ExcludedRoutes lists (Provider, Model, Endpoint) combinations the caller
    // has determined are currently unavailable. The router skips any candidate
    // matching an entry. Provider is required; Model and Endpoint are optional
    // (empty matches any value for that field).
    //
    // Use this to communicate caller-side health signals across calls without
    // redesigning provider config. The routing engine records excluded
    // candidates with FilterReasonCallerExcluded for observability.
    ExcludedRoutes []ExcludedRoute
}
```


### `RouteDecision`

RouteDecision is the result of ResolveRoute.

```go
type RouteDecision struct {
    // RequestedPolicy is the caller-supplied policy, when any.
    RequestedPolicy string
    // SnapshotCapturedAt records when the model snapshot used for scoring
    // was assembled.
    SnapshotCapturedAt time.Time
    // PowerPolicy records the effective policy inputs used for this
    // resolution. It stays separate from the chosen model so operator
    // surfaces can explain policy without re-deriving it.
    PowerPolicy RoutePowerPolicy
    // Harness is the selected harness name.
    Harness string
    // Provider is the selected provider for native agent routes.
    Provider string
    // Endpoint is the selected named endpoint when the provider exposes more
    // than one endpoint.
    Endpoint string
    // ServerInstance is the normalized server identity used for sticky
    // affinity and route evidence.
    ServerInstance string
    // Model is the selected concrete model.
    Model string
    // Reason summarizes why the selected candidate won.
    Reason string
    // Sticky captures whether this decision reused an existing sticky lease
    // or created a new one.
    Sticky RouteStickyState
    // Utilization captures the endpoint sample that informed the selected
    // candidate, when known.
    Utilization RouteUtilizationState
    // Power is the catalog-projected power of the selected Model
    // (per CONTRACT-003 § Catalog Power Projection). 0 means
    // unknown/exact-pin-only/no catalog entry. DDx callers read this
    // to compute next-attempt MinPower without importing catalog code.
    Power int
    // Candidates is the full ranked decision trace, including rejected
    // candidates and their rejection reasons.
    Candidates []RouteCandidate
}
```



## Core functions


### `New`

New constructs a FizeauService.

```go
func New(opts ServiceOptions) (FizeauService, error)
```


### `RegisterConfigLoader`

RegisterConfigLoader is called by the config package's init() to install the.

```go
func RegisterConfigLoader(fn func(dir string) (ServiceConfig, error))
```



## Full exported surface

Generated from `go/doc` over the root package. For full doc comments,
parameter types, and field-level documentation, follow the
[pkg.go.dev](https://pkg.go.dev/github.com/easel/fizeau) link.

| Name | Kind | Synopsis |
|------|------|---------|
| `AccountStatus` | type (struct) | AccountStatus describes authentication/account state without exposing |
| `AnchorStore` | type (alias) |  |
| `AvailableHarnesses` | func (func AvailableHarnesses() []string) | AvailableHarnesses returns the canonical names of every harness whose CLI |
| `BashOutputFilterConfig` | type (alias) |  |
| `BillingModel` | type (alias) |  |
| `BillingModelFixed` | const |  |
| `BillingModelPerToken` | const |  |
| `BillingModelSubscription` | const |  |
| `BillingModelUnknown` | const |  |
| `CachePolicyDefault` | const | Valid CachePolicy values |
| `CachePolicyOff` | const | Valid CachePolicy values |
| `CatalogProbeFunc` | type (func) | CatalogProbeFunc performs a single /v1/models discovery request against a |
| `CatalogResult` | type (struct) | CatalogResult is what callers receive from Get |
| `CompactionConfig` | type (alias) |  |
| `ContextSourceCatalog` | const |  |
| `ContextSourceDefault` | const |  |
| `ContextSourceProviderAPI` | const |  |
| `ContextSourceProviderConfig` | const |  |
| `ContextSourceUnknown` | const |  |
| `CooldownState` | type (struct) | CooldownState describes an active routing cooldown for a provider |
| `CorrelationIDNormalizationError` | type (struct) | CorrelationIDNormalizationError is returned pre-dispatch when |
| `CostInfo` | type (struct) | CostInfo holds per-token cost metadata for a model |
| `DecisionWithCandidates` | type (interface) | DecisionWithCandidates is implemented by routing errors that retain the |
| `DecodeServiceEvent` | func (func DecodeServiceEvent(ev ServiceEvent) (ServiceDecodedEvent, error)) |  |
| `DiscoverModels` | func (func DiscoverModels(ctx context.Context, baseURL, apiKey string) ([]string, error)) |  |
| `DrainExecute` | func (func DrainExecute(ctx context.Context, events <-chan ServiceEvent) (*DrainExecuteResult, error)) |  |
| `DrainExecuteResult` | type (struct) | DrainExecuteResult is a typed aggregate of one Execute event stream |
| `EndpointStatus` | type (struct) | EndpointStatus describes one configured provider endpoint probe |
| `ErrDiscoveryUnsupported` | func (func ErrDiscoveryUnsupported() error) | ErrDiscoveryUnsupported returns the sentinel |
| `ErrHarnessModelIncompatible` | type (struct) | ErrHarnessModelIncompatible reports an explicit Harness+Model pin that the |
| `ErrModelConstraintAmbiguous` | type (struct) | ErrModelConstraintAmbiguous reports that an explicit Model pin matched more |
| `ErrModelConstraintNoMatch` | type (struct) | ErrModelConstraintNoMatch reports that an explicit Model pin matched no |
| `ErrNoLiveProvider` | type (struct) | ErrNoLiveProvider reports that profile-tier escalation walked the entire |
| `ErrPolicyRequirementUnsatisfied` | type (struct) | ErrPolicyRequirementUnsatisfied reports a hard policy requirement that no |
| `ErrRejectedOverride` | type (struct) | ErrRejectedOverride wraps a pin-rejection error and carries the |
| `ErrUnknownPolicy` | type (struct) | ErrUnknownPolicy reports an explicit policy name that is not present in the |
| `ErrUnknownProvider` | type (struct) | ErrUnknownProvider reports an explicit Provider pin that is not present in |
| `ErrUnsatisfiablePin` | type (struct) | ErrUnsatisfiablePin reports explicit caller pins that cannot all be true at |
| `EventSessionEnd` | const |  |
| `EventSessionStart` | const |  |
| `ExcludedRoute` | type (struct) | ExcludedRoute identifies a (Provider, Model, optional Endpoint) combination |
| `FilterReasonContextTooSmall` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonEndpointUnreachable` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonMeteredOptInRequired` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonNoToolSupport` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonProviderExcludedFromDefault` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonReasoningUnsupported` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonScoredBelowTop` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FilterReasonUnhealthy` | const | FilterReason* enumerate the canonical disqualification reasons surfaced |
| `FizeauService` | type (interface) | FizeauService is the entire public Go surface of the fizeau module |
| `HarnessAvailability` | type (struct) | HarnessAvailability reports the install status of a single harness CLI as |
| `HarnessCapability` | type (struct) | HarnessCapability describes one capability row for one harness |
| `HarnessCapabilityMatrix` | type (struct) | HarnessCapabilityMatrix is the public, per-harness capability table exposed |
| `HarnessCapabilityStatus` | type | HarnessCapabilityStatus classifies one harness capability in the public |
| `HarnessInfo` | type (struct) | HarnessInfo describes a registered harness as defined in CONTRACT-003 |
| `HealthTarget` | type (struct) | HealthTarget identifies what to health-check |
| `MetadataKeyCorrelationID` | const | Reserved cross-tool metadata keys (CONTRACT-003 § ExecuteRequest |
| `MetadataKeyRole` | const | Reserved cross-tool metadata keys (CONTRACT-003 § ExecuteRequest |
| `MetadataWarningCodeKeyCollision` | const | MetadataWarningCodeKeyCollision is the ServiceFinalWarning |
| `ModelFilter` | type (struct) | ModelFilter filters ListModels results |
| `ModelInfo` | type (struct) | ModelInfo describes a model with full metadata per CONTRACT-003 |
| `New` | func (func New(opts ServiceOptions) (FizeauService, error)) | New constructs a FizeauService |
| `NoViableProviderForNow` | type (struct) | NoViableProviderForNow reports that every otherwise-eligible routing |
| `NormalizeModelID` | func (func NormalizeModelID(requested string, catalog []string) (string, error)) |  |
| `OverrideClassBucket` | type (struct) | OverrideClassBucket is one cell in the override-class pivot |
| `PerfSignal` | type (struct) | PerfSignal holds observed performance data for a model |
| `PolicyInfo` | type (struct) |  |
| `ProviderAlivenessProber` | type (func) | ProviderAlivenessProber reports whether a provider endpoint is reachable |
| `ProviderBurnRateTracker` | type (struct) | ProviderBurnRateTracker maintains a per-provider rolling window of token |
| `ProviderInfo` | type (struct) | ProviderInfo describes a provider with live status per CONTRACT-003 |
| `ProviderQuotaState` | type | ProviderQuotaState is the state of one provider in the quota state machine |
| `ProviderQuotaStateStore` | type (struct) | ProviderQuotaStateStore is the per-provider quota state machine |
| `QuotaRecoveryProber` | type (func) | QuotaRecoveryProber reports whether a quota_exhausted provider has recovered |
| `QuotaState` | type (struct) | QuotaState is a live quota snapshot for a harness |
| `RankModels` | func (func RankModels(candidates []string, knownModels map[string]string, pattern string) ([]ScoredModel, error)) |  |
| `ReadSessionEvents` | func (func ReadSessionEvents(path string) ([]SessionEvent, error)) |  |
| `ReadTool` | type (alias) |  |
| `Reasoning` | type (alias) |  |
| `ReasoningAuto` | const |  |
| `ReasoningHigh` | const |  |
| `ReasoningLow` | const |  |
| `ReasoningMax` | const |  |
| `ReasoningMedium` | const |  |
| `ReasoningMinimal` | const |  |
| `ReasoningOff` | const |  |
| `ReasoningXHigh` | const |  |
| `RegisterConfigLoader` | func (func RegisterConfigLoader(fn func(dir string) (ServiceConfig, error))) | RegisterConfigLoader is called by the config package's init() to install the |
| `RoleNormalizationError` | type (struct) | RoleNormalizationError is returned pre-dispatch when ServiceExecuteRequest |
| `RouteAttempt` | type (struct) | RouteAttempt is caller feedback about one attempted route candidate |
| `RouteCandidate` | type (struct) | RouteCandidate is one routing candidate evaluated by ResolveRoute |
| `RouteCandidateComponents` | type (struct) | RouteCandidateComponents breaks down the inputs that fed a candidate's |
| `RouteCandidateStatus` | type (struct) | RouteCandidateStatus describes a single live provider/model candidate |
| `RouteDecision` | type (struct) | RouteDecision is the result of ResolveRoute |
| `RoutePowerPolicy` | type (struct) | RoutePowerPolicy captures the numeric power-policy inputs associated with |
| `RouteRequest` | type (struct) | RouteRequest specifies a routing query |
| `RouteStatusEntry` | type (struct) | RouteStatusEntry describes the live provider candidates serving one model |
| `RouteStatusReport` | type (struct) | RouteStatusReport is returned by RouteStatus |
| `RouteStatusRoutingQualityWindow` | const | RouteStatusRoutingQualityWindow caps how many recent Execute calls |
| `RouteStickyState` | type (struct) | RouteStickyState describes sticky routing evidence without exposing the |
| `RouteUtilizationState` | type (struct) | RouteUtilizationState summarizes the live utilization sample associated |
| `RoutingQualityMetrics` | type (struct) | RoutingQualityMetrics is the bundle of three first-class metrics ADR-006 |
| `ScanSkillsDir` | func (func ScanSkillsDir(dir string) (*SkillCatalog, []string, error)) | ScanSkillsDir walks dir for SKILL |
| `ScoredModel` | type (alias) |  |
| `ServiceCompactionData` | type (struct) |  |
| `ServiceConfig` | type (interface) | ServiceConfig provides provider and routing data to the service without |
| `ServiceConfigSource` | type (interface) | ServiceConfigSource is the minimal interface providerCooldownsFromSnapshotErrors |
| `ServiceDecodedEvent` | type (struct) | ServiceDecodedEvent is a typed view of one ServiceEvent |
| `ServiceEvent` | type (alias) | ServiceEvent is a contract-level event (mirrors harnesses |
| `ServiceEventTypeCompaction` | const |  |
| `ServiceEventTypeFinal` | const |  |
| `ServiceEventTypeOverride` | const |  |
| `ServiceEventTypeProgress` | const |  |
| `ServiceEventTypeRejectedOverride` | const |  |
| `ServiceEventTypeRoutingDecision` | const |  |
| `ServiceEventTypeStall` | const |  |
| `ServiceEventTypeTextDelta` | const |  |
| `ServiceEventTypeToolCall` | const |  |
| `ServiceEventTypeToolResult` | const |  |
| `ServiceExecuteRequest` | type (struct) | ServiceExecuteRequest is the public ExecuteRequest type per CONTRACT-003 |
| `ServiceFinalData` | type (struct) |  |
| `ServiceFinalUsage` | type (struct) | ServiceFinalUsage is the public token-usage payload emitted on service |
| `ServiceFinalWarning` | type (struct) |  |
| `ServiceOptions` | type (struct) | ServiceOptions configures a FizeauService instance |
| `ServiceOverrideAutoComponents` | type (struct) | ServiceOverrideAutoComponents mirrors RouteCandidateComponents for the |
| `ServiceOverrideData` | type (struct) | ServiceOverrideData is the payload for both override and rejected_override |
| `ServiceOverrideOutcome` | type (struct) | ServiceOverrideOutcome carries post-execution status mirrored from the |
| `ServiceOverridePin` | type (struct) | ServiceOverridePin captures a (harness, provider, model) tuple, used both |
| `ServiceOverridePromptFeatures` | type (struct) | ServiceOverridePromptFeatures captures prompt-classification inputs that |
| `ServiceProgressData` | type (struct) | ServiceProgressData is the bounded progress payload emitted alongside the |
| `ServiceProviderEndpoint` | type (struct) | ServiceProviderEndpoint is one configured provider serving endpoint |
| `ServiceProviderEntry` | type (struct) | ServiceProviderEntry carries the minimal provider data the service needs |
| `ServiceRoutingActual` | type (struct) |  |
| `ServiceRoutingDecisionCandidate` | type (struct) | ServiceRoutingDecisionCandidate is one entry in the routing-decision |
| `ServiceRoutingDecisionComponents` | type (struct) | ServiceRoutingDecisionComponents exposes the per-axis score inputs |
| `ServiceRoutingDecisionData` | type (struct) |  |
| `ServiceRoutingStickyState` | type (struct) |  |
| `ServiceRoutingUtilizationState` | type (struct) |  |
| `ServiceStallData` | type (struct) |  |
| `ServiceTextDeltaData` | type (struct) |  |
| `ServiceToolCallData` | type (struct) |  |
| `ServiceToolResultData` | type (struct) |  |
| `ServiceUsageSourceEvidence` | type (struct) |  |
| `ServiceUsageTokenCounts` | type (struct) |  |
| `SessionEndData` | type (alias) |  |
| `SessionEvent` | type (alias) |  |
| `SessionEventType` | type (alias) |  |
| `SessionLogEntry` | type (struct) | SessionLogEntry describes one historical session log file projected from the |
| `SessionLogger` | type (alias) | SessionLogger writes session log events |
| `SessionStartData` | type (alias) |  |
| `SessionStatus` | type (alias) |  |
| `SkillCatalog` | type (alias) |  |
| `StallPolicy` | type (struct) | StallPolicy bounds how long the agent will spin without making progress |
| `StatusError` | type (struct) | StatusError describes the most recent normalized status error for a harness, |
| `StatusSuccess` | const |  |
| `SubscriptionCostCurve` | type (struct) | SubscriptionCostCurve tunes effective subscription cost by quota utilization |
| `TokenUsage` | type (alias) |  |
| `Tool` | type (alias) |  |
| `UsageReport` | type (struct) | UsageReport is the public, service-owned aggregation over historical session |
| `UsageReportOptions` | type (struct) | UsageReportOptions configures FizeauService |
| `UsageReportRow` | type (struct) | UsageReportRow aggregates usage for one provider/model pair |
| `UsageReportWindow` | type (struct) | UsageReportWindow describes the active reporting window |
| `UsageWindow` | type (struct) | UsageWindow describes normalized usage attribution over a time window |
| `ValidateCachePolicy` | func (func ValidateCachePolicy(v string) error) | ValidateCachePolicy returns nil when v is one of the accepted CachePolicy |
| `ValidateCorrelationID` | func (func ValidateCorrelationID(id string) error) | ValidateCorrelationID returns nil when id is empty or normalized, and a |
| `ValidatePowerBounds` | func (func ValidatePowerBounds(minPower, maxPower int) error) | ValidatePowerBounds returns nil when the optional numeric routing power |
| `ValidateRole` | func (func ValidateRole(role string) error) | ValidateRole returns nil when role is empty (unset) or already |
| `ValidateUsageSince` | func (func ValidateUsageSince(spec string) error) | ValidateUsageSince returns nil when spec is a usage window value accepted by |
| `alivenessEndpoint` | type (struct) | alivenessEndpoint describes one provider endpoint to probe |
| `alivenessLoopSleep` | func (func alivenessLoopSleep(ctx context.Context, d time.Duration) bool) |  |
| `anyProviderSupportsTools` | func (func anyProviderSupportsTools(providers []routing.ProviderEntry) bool) |  |
| `appendUniqueModelIDs` | func (func appendUniqueModelIDs(values []string, additions ...string) []string) |  |
| `applyRouteSnapshotEvidence` | func (func applyRouteSnapshotEvidence(candidate *RouteCandidate, row modelsnapshot.KnownModel)) |  |
| `applyRouteSnapshotEvidenceToStatus` | func (func applyRouteSnapshotEvidenceToStatus(candidate *RouteCandidateStatus, row modelsnapshot.KnownModel)) |  |
| `assembleModelSnapshotFromServiceConfig` | func (func assembleModelSnapshotFromServiceConfig(ctx context.Context, sc ServiceConfig, cat *modelcatalog.Catalog, cacheRoot string) (modelsnapshot.ModelSnapshot, error)) |  |
| `assembleModelSnapshotFromServiceConfigWithOptions` | func (func assembleModelSnapshotFromServiceConfigWithOptions(ctx context.Context, sc ServiceConfig, cat *modelcatalog.Catalog, cacheRoot string, opts modelsnapshot.AssembleOptions) (modelsnapshot.ModelSnapshot, error)) |  |
| `axesOverridden` | func (func axesOverridden(req ServiceExecuteRequest) []string) | axesOverridden returns the canonical, ordered list of axes the caller |
| `boundedProgressText` | func (func boundedProgressText(s string, maxRunes int) string) |  |
| `buildProviderContextWindows` | func (func buildProviderContextWindows(ctx context.Context, pcfg ServiceProviderEntry, cat *modelcatalog.Catalog, discoveredIDs []string) (map[string]int, map[string]string)) | buildProviderContextWindows assembles the ContextWindows map for a |
| `candidateProviderIdentity` | func (func candidateProviderIdentity(h routing.HarnessEntry, p routing.ProviderEntry) string) |  |
| `canonicalHarnessPin` | func (func canonicalHarnessPin(harness string) string) |  |
| `capabilityScoreForCostClass` | func (func capabilityScoreForCostClass(class string) float64) | capabilityScoreForCostClass maps the harness cost class to a coarse |
| `catalogCache` | type (struct) | catalogCache is the service-scope live-catalog cache with stale-while- |
| `catalogCacheKey` | type (struct) | catalogCacheKey identifies the cache entry |
| `catalogCacheOptions` | type (struct) | catalogCacheOptions controls timings + test hooks |
| `catalogCostAndPerf` | func (func catalogCostAndPerf(cat *modelcatalog.Catalog, modelID string) (CostInfo, PerfSignal)) | catalogCostAndPerf extracts CostInfo and PerfSignal for a model from the catalog |
| `catalogCostUSDPer1kTokens` | func (func catalogCostUSDPer1kTokens(cat *modelcatalog.Catalog, modelID string) (float64, bool)) |  |
| `catalogEntry` | type (struct) | catalogEntry is the per-key cached state |
| `catalogPowerEligibility` | func (func catalogPowerEligibility(cat *modelcatalog.Catalog, modelID string) (int, bool, bool)) |  |
| `catalogPowerForModel` | func (func catalogPowerForModel(cat *modelcatalog.Catalog, modelID string) int) | catalogPowerForModel returns the catalog-projected power for a model |
| `claudeCLIExecutableModel` | func (func claudeCLIExecutableModel(model string) string) |  |
| `cloneStringMap` | func (func cloneStringMap(src map[string]string) map[string]string) |  |
| `collectConcreteModelCandidates` | func (func collectConcreteModelCandidates(reqHarness, reqProvider, reqModel string, in routing.Inputs, cat *modelcatalog.Catalog) []string) |  |
| `collectDefaultModelCandidates` | func (func collectDefaultModelCandidates(reqHarness, reqProvider string, in routing.Inputs) []string) |  |
| `compactProgressIdentity` | func (func compactProgressIdentity(taskID string, round int) string) |  |
| `compactProgressTaskID` | func (func compactProgressTaskID(taskID string) string) |  |
| `compactionAssertionHookFn` | type (func) |  |
| `contextWindowSourceForProviderConfig` | func (func contextWindowSourceForProviderConfig(pcfg ServiceProviderEntry) string) |  |
| `convertUsageWindow` | func (func convertUsageWindow(w *UsageReportWindow) *session.UsageWindow) |  |
| `correlationIDMaxLen` | const | CorrelationID normalization bounds (CONTRACT-003) |
| `cryptoRandInt63n` | func (func cryptoRandInt63n(n int64) int64) | cryptoRandInt63n uses crypto/rand for the jitter randomization |
| `decodeServicePayload` | func (func decodeServicePayload(ev ServiceEvent, dst any) error) |  |
| `defaultCatalogAsyncRefreshTimeout` | const | Default cache timings |
| `defaultCatalogFreshTTL` | const | Default cache timings |
| `defaultCatalogProbeTimeout` | const |  |
| `defaultCatalogReloadTimeout` | const |  |
| `defaultCatalogStaleTTL` | const | Default cache timings |
| `defaultCatalogUnreachableCooldown` | const | Default cache timings |
| `defaultCatalogUnreachableJitter` | const | Default cache timings |
| `defaultHarnessInstances` | func (func defaultHarnessInstances() map[string]harnesses.Harness) | defaultHarnessInstances returns the production map of registered |
| `defaultHealthProbeInterval` | const |  |
| `defaultHealthSignalTTL` | const |  |
| `defaultQuotaRecoveryFallbackInterval` | const |  |
| `defaultQuotaRefreshDebounce` | const |  |
| `defaultQuotaRefreshProbeTimeout` | const |  |
| `defaultQuotaRefreshStartupWait` | const |  |
| `defaultRouteHealthPath` | func (func defaultRouteHealthPath(sc ServiceConfig) string) |  |
| `defaultStaleHarnessReaperGrace` | const |  |
| `derefHarnessInt` | func (func derefHarnessInt(v *int) int) |  |
| `discoveryUnsupportedError` | type (struct) |  |
| `earliestQuotaResetAfter` | func (func earliestQuotaResetAfter(windows []harnesses.QuotaWindow, now time.Time) time.Time) |  |
| `effectiveReasoningString` | func (func effectiveReasoningString(value Reasoning) string) |  |
| `emitFatalFinal` | func (func emitFatalFinal(out chan<- ServiceEvent, meta map[string]string, status, errMsg string)) | emitFatalFinal is used when Execute itself can't construct a route |
| `emitFinal` | func (func emitFinal(out chan<- ServiceEvent, seq *atomic.Int64, meta map[string]string, final harnesses.FinalData)) | emitFinal wraps a FinalData into a ServiceEvent and writes it to out |
| `emitJSON` | func (func emitJSON(out chan<- ServiceEvent, seq *atomic.Int64, t harnesses.EventType, meta map[string]string, payload any)) | emitJSON marshals payload and writes a typed event to out |
| `emitJSONRaw` | func (func emitJSONRaw(out chan<- ServiceEvent, seq *atomic.Int64, t harnesses.EventType, meta map[string]string, payload any)) | emitJSONRaw is the typed-payload variant used inside the loop callback |
| `emitProgress` | func (func emitProgress(out chan<- ServiceEvent, seq *atomic.Int64, sl *serviceSessionLog, sessionID string, meta map[string]string, payload ServiceProgressData)) |  |
| `endpointDisplayName` | func (func endpointDisplayName(name, baseURL string) string) |  |
| `endpointProviderRef` | func (func endpointProviderRef(providerName, endpointName string) string) |  |
| `endpointStatus` | func (func endpointStatus(status string) string) |  |
| `errDialishNetworkError` | var | errDialishNetworkError is a sentinel for errors |
| `errDiscoveryUnsupported` | var | errDiscoveryUnsupported is the sentinel for "endpoint exists but /v1/models |
| `errHarnessModelIncompatible` | var |  |
| `errModelConstraintAmbiguous` | var |  |
| `errModelConstraintNoMatch` | var |  |
| `errNoLiveProvider` | var |  |
| `errNoViableProviderForNow` | var |  |
| `errPolicyRequirementUnsatisfied` | var |  |
| `errUnknownPolicy` | var |  |
| `errUnknownProvider` | var |  |
| `errUnsatisfiablePin` | var |  |
| `escalatePolicyLadder` | func (func escalatePolicyLadder(req routing.Request, in routing.Inputs, origErr error, displayPolicy string) (bool, *routing.Decision, error)) | escalatePolicyLadder walks routing |
| `executeEventFanout` | type (interface) |  |
| `executeRouteResolver` | type (interface) |  |
| `executeRunContext` | type (struct) |  |
| `executeRunnerInvoker` | type (interface) |  |
| `executeRunnerRequest` | func (func executeRunnerRequest(req ServiceExecuteRequest, decision RouteDecision, meta map[string]string, start time.Time) serviceimpl.ExecuteRunnerRequest) |  |
| `executeSessionLogOpener` | type (interface) |  |
| `explicitPolicyConstraint` | func (func explicitPolicyConstraint(policy string) (string, bool)) |  |
| `explicitQuotaUnavailable` | func (func explicitQuotaUnavailable(name string, windows []harnesses.QuotaWindow, now time.Time) error) |  |
| `extractHostPort` | func (func extractHostPort(baseURL string) string) | extractHostPort extracts host:port from a base URL, adding the scheme's |
| `extractStatusCode` | func (func extractStatusCode(msg string) int) | extractStatusCode pulls the status code out of the "HTTP NNN:" prefix |
| `fillProgressIdentity` | func (func fillProgressIdentity(payload *ServiceProgressData, sessionID string, meta map[string]string)) |  |
| `finalUsageToCoreTokens` | func (func finalUsageToCoreTokens(usage *harnesses.FinalUsage) agentcore.TokenUsage) | finalUsageToCoreTokens converts the public FinalUsage pointer form into |
| `finalizeAndEmit` | func (func finalizeAndEmit(out chan<- ServiceEvent, seq *atomic.Int64, meta map[string]string, req ServiceExecuteRequest, sl *serviceSessionLog, final harnesses.FinalData)) | finalizeAndEmit stamps the service-owned session-log path onto final, |
| `formatByteCount` | func (func formatByteCount(n int) string) |  |
| `generateSessionID` | func (func generateSessionID() string) | generateSessionID returns a unique session identifier for a new Execute |
| `harnessInstanceHook` | var | harnessInstanceHook, when non-nil, is applied to the default harness map |
| `harnessRunsInProcessOrHTTP` | func (func harnessRunsInProcessOrHTTP(cfg harnesses.HarnessConfig) bool) |  |
| `harnessSource` | func (func harnessSource(req ServiceExecuteRequest) string) |  |
| `harnessStatusToCoreStatus` | func (func harnessStatusToCoreStatus(status string) agentcore.Status) | harnessStatusToCoreStatus maps a public harnesses |
| `harnessType` | func (func harnessType(cfg harnesses.HarnessConfig) string) | harnessType returns "native" for HTTP/embedded harnesses, "subprocess" for CLI-invoked ones |
| `hashHeaders` | func (func hashHeaders(headers map[string]string) [sha256.Size]byte) | hashHeaders fingerprints a headers map in a deterministic way so ordering |
| `healthCheckQuotaProbeMu` | var |  |
| `hexDigit` | func (func hexDigit(b byte) byte) |  |
| `isDiscoveryUnsupported` | func (func isDiscoveryUnsupported(err error) bool) |  |
| `isDispatchabilityFailure` | func (func isDispatchabilityFailure(errMsg string) bool) |  |
| `isExplicitPinError` | func (func isExplicitPinError(err error) bool) |  |
| `isNetworkFailure` | func (func isNetworkFailure(err error) bool) | isNetworkFailure returns true when err looks like a transport-level |
| `isReachabilityErr` | func (func isReachabilityErr(err error) bool) | isReachabilityErr reports whether err carries the openai |
| `isSensitiveSummaryKey` | func (func isSensitiveSummaryKey(key string) bool) |  |
| `isServerError` | func (func isServerError(msg string) bool) | isServerError returns true when the error message indicates a 5xx |
| `isSnapshotDialFailure` | func (func isSnapshotDialFailure(errMsg string) bool) | isSnapshotDialFailure preserved as a back-compat alias for the v0 |
| `lastDecisionEntry` | type (struct) |  |
| `loadRoutingCatalog` | var |  |
| `loadServiceConfig` | var | loadServiceConfig, when non-nil, is called by New to load a ServiceConfig |
| `makeOverrideEvent` | func (func makeOverrideEvent(ovr *overrideContext, sessionID string, finalEv ServiceEvent, meta map[string]string) (ServiceEvent, ServiceOverrideData, bool)) | makeOverrideEvent constructs the wire-level override event, stamping |
| `makeRejectedOverrideEvent` | func (func makeRejectedOverrideEvent(ovr *overrideContext, sessionID string, pinErr error, meta map[string]string) (ServiceEvent, ServiceOverrideData, bool)) | makeRejectedOverrideEvent constructs a rejected_override event from the |
| `maxQuotaWindowUsedPercent` | func (func maxQuotaWindowUsedPercent(windows []harnesses.QuotaWindow) float64) |  |
| `metaWithRoleAndCorrelation` | func (func metaWithRoleAndCorrelation(meta map[string]string, role, correlationID string) map[string]string) | metaWithRoleAndCorrelation overlays the caller-supplied top-level Role |
| `metadataKeyCollisionMessage` | func (func metadataKeyCollisionMessage(keys []string) string) | metadataKeyCollisionMessage formats a human-readable message for the |
| `metadataReservedKeyCollisions` | func (func metadataReservedKeyCollisions(meta map[string]string, role, correlationID string) []string) | metadataReservedKeyCollisions returns the reserved metadata keys whose |
| `modelDiscoveryEndpoint` | type (struct) |  |
| `modelSnapshotProviderConfig` | func (func modelSnapshotProviderConfig(entry ServiceProviderEntry) modelsnapshot.ProviderConfig) |  |
| `modelSupportedForHarness` | func (func modelSupportedForHarness(name string, cfg harnesses.HarnessConfig, model, provider string) bool) |  |
| `modelSupportsToolsByID` | func (func modelSupportsToolsByID(cat *modelcatalog.Catalog, modelIDs []string) map[string]bool) |  |
| `nativeCompactionPayload` | type (alias) |  |
| `nativeDecision` | func (func nativeDecision(decision RouteDecision) serviceimpl.NativeDecision) |  |
| `nativeLLMRequestPayload` | type (alias) |  |
| `nativeLLMResponsePayload` | type (alias) |  |
| `nativeModelReasoningWireMap` | func (func nativeModelReasoningWireMap() map[string]string) | nativeModelReasoningWireMap returns the catalog reasoning_wire map for use |
| `nativeProgressState` | type (struct) |  |
| `nativeProviderResolution` | type (struct) |  |
| `nativeRouteCandidates` | func (func nativeRouteCandidates(in []RouteCandidate) []serviceimpl.NativeRouteCandidate) |  |
| `nativeToolCallPayload` | type (alias) |  |
| `nativeToolsForRequest` | func (func nativeToolsForRequest(req ServiceExecuteRequest) []agentcore.Tool) |  |
| `newServiceCompactor` | func (func newServiceCompactor(req ServiceExecuteRequest, model string) agentcore.Compactor) |  |
| `newSessionHub` | func (func newSessionHub() *serviceimpl.SessionHub) |  |
| `nextQuotaRecoveryBackoff` | func (func nextQuotaRecoveryBackoff(prev time.Duration) time.Duration) | nextQuotaRecoveryBackoff returns the next bounded backoff value: doubles the |
| `normalizeServiceProviderType` | func (func normalizeServiceProviderType(t string) string) |  |
| `normalizeShellCommand` | func (func normalizeShellCommand(command string) string) |  |
| `overrideAxisHarness` | const | overrideAxis* enumerates the three independently-tracked override axes |
| `overrideAxisModel` | const | overrideAxis* enumerates the three independently-tracked override axes |
| `overrideAxisProvider` | const | overrideAxis* enumerates the three independently-tracked override axes |
| `overrideContext` | type (struct) | overrideContext carries the per-Execute override-event payload from the |
| `overrideReasonHint` | func (func overrideReasonHint(req ServiceExecuteRequest) string) | overrideReasonHint returns the caller-supplied free-form reason from |
| `policyForName` | func (func policyForName(cat *modelcatalog.Catalog, name string) (modelcatalog.Policy, string, bool)) |  |
| `positiveScorePart` | func (func positiveScorePart(v float64) float64) |  |
| `primaryQuotaRefresh` | var |  |
| `probeOpenAIModels` | func (func probeOpenAIModels(ctx context.Context, baseURL, apiKey string) ([]string, error)) | probeOpenAIModels calls GET /v1/models against baseURL and classifies |
| `probeServiceProvider` | func (func probeServiceProvider(ctx context.Context, entry ServiceProviderEntry) (status string, modelCount int, caps []string)) | probeServiceProvider pings a provider and returns (status, modelCount, capabilities) |
| `processGroupAlive` | func (func processGroupAlive(pgid int) bool) |  |
| `processOutcomeForFinal` | func (func processOutcomeForFinal(status string) string) | processOutcomeForFinal returns the FEAT-005 §27 process_outcome label for |
| `progressExceptionalLineLimit` | const |  |
| `progressLineLimit` | const |  |
| `progressMessageLimit` | func (func progressMessageLimit(payload ServiceProgressData) int) |  |
| `progressStatusLine` | func (func progressStatusLine(payload ServiceProgressData) string) |  |
| `progressTaskID` | func (func progressTaskID(sessionID string, meta map[string]string) string) |  |
| `progressTokenThroughput` | func (func progressTokenThroughput(outputTokens int, durationMS int64) *float64) |  |
| `promptAssertionHookFn` | type (func) | promptAssertionHookFn / compactionAssertionHookFn / toolWiringHookFn |
| `providerCapabilities` | func (func providerCapabilities(entry ServiceProviderEntry) []string) | providerCapabilities returns the capability set for a provider entry |
| `providerCooldownsFromSnapshotErrors` | func (func providerCooldownsFromSnapshotErrors(snapshot modelsnapshot.ModelSnapshot, cfg ServiceConfigSource, now time.Time, ttl time.Duration) map[string]time.Time) | providerCooldownsFromSnapshotErrors walks snapshot |
| `providerPreferenceForPolicy` | func (func providerPreferenceForPolicy(cat *modelcatalog.Catalog, policy string) (string, error)) |  |
| `providerPreferenceForPolicyName` | func (func providerPreferenceForPolicyName(name string) string) |  |
| `providerProbePriority` | func (func providerProbePriority(status string) int) |  |
| `providerProbeResult` | type (struct) |  |
| `providerRoutingCostClass` | func (func providerRoutingCostClass(providerType string) string) |  |
| `providerSupportsTools` | func (func providerSupportsTools(cat *modelcatalog.Catalog, defaultModel string, discoveredIDs []string) bool) | providerSupportsTools returns whether the provider should be advertised as |
| `providerTypeUsesFixedBilling` | func (func providerTypeUsesFixedBilling(providerType string) bool) |  |
| `providerUsesLiveDiscovery` | func (func providerUsesLiveDiscovery(providerType string) bool) |  |
| `publicFilterReason` | func (func publicFilterReason(c routing.Candidate) string) | publicFilterReason maps the typed FilterReason emitted by the internal |
| `publicRoutingError` | func (func publicRoutingError(err error, candidates []RouteCandidate, requestedPolicy ...string) error) |  |
| `publicToRoutingExcludedRoutes` | func (func publicToRoutingExcludedRoutes(in []ExcludedRoute) []routing.ExcludedRoute) |  |
| `quotaCacheStatus` | type (struct) |  |
| `quotaRecoveryBackoffInitial` | const |  |
| `quotaRecoveryBackoffMax` | const |  |
| `quotaRecoverySleep` | func (func quotaRecoverySleep(ctx context.Context, d time.Duration) bool) | quotaRecoverySleep blocks for d or until ctx is cancelled |
| `quotaRefreshCoordinator` | type (struct) |  |
| `quotaRefreshMode` | type |  |
| `quotaRefreshPolicy` | type (struct) |  |
| `quotaStatus` | func (func quotaStatus(fresh bool, windows []harnesses.QuotaWindow) string) |  |
| `quotaTrend` | func (func quotaTrend(percentUsed int, fresh bool) string) |  |
| `reapStaleHarnessRecords` | func (func reapStaleHarnessRecords(dir string, grace time.Duration, now time.Time) error) |  |
| `recordRouteTimeProbeFailures` | func (func recordRouteTimeProbeFailures(store *routehealth.ProbeStore, endpoints []alivenessEndpoint, probeAt time.Time)) |  |
| `requestPrimaryQuotaRefresh` | func (func requestPrimaryQuotaRefresh(ctx context.Context, harnessName string, policy quotaRefreshPolicy, harnessByName func(string) harnesses.Harness) <-chan struct{}) |  |
| `requestedNativeProviderType` | func (func requestedNativeProviderType(req ServiceExecuteRequest) string) |  |
| `resolveCatalogCostModel` | func (func resolveCatalogCostModel(cat *modelcatalog.Catalog, ref string) string) |  |
| `resolveContextEvidence` | func (func resolveContextEvidence(ctx context.Context, entry ServiceProviderEntry, modelID string, cat *modelcatalog.Catalog) (int, string)) | resolveContextEvidence resolves the context window for a model using the |
| `resolveSingleModelMatch` | func (func resolveSingleModelMatch(reqModel string, candidates []string) (string, error)) |  |
| `resolveSubprocessModelAlias` | func (func resolveSubprocessModelAlias(harness, model string) string) |  |
| `roleMaxLen` | const | Role normalization bounds (CONTRACT-003) |
| `routeDecisionError` | type (struct) |  |
| `routePowerBoundsForRequest` | func (func routePowerBoundsForRequest(req RouteRequest, policy RoutePowerPolicy) (int, int)) |  |
| `routeSnapshotCandidateIndex` | func (func routeSnapshotCandidateIndex(snapshot modelsnapshot.ModelSnapshot) map[routeSnapshotCandidateKey]modelsnapshot.KnownModel) |  |
| `routeSnapshotCandidateKey` | type (struct) |  |
| `routeSnapshotEvidenceForCandidate` | func (func routeSnapshotEvidenceForCandidate(candidate RouteCandidate, snapshot modelsnapshot.ModelSnapshot) (modelsnapshot.KnownModel, bool)) |  |
| `routeStatusMetricKey` | func (func routeStatusMetricKey(provider, endpoint, model string) string) |  |
| `routeStatusMetricValue` | func (func routeStatusMetricValue(values map[string]float64, provider, endpoint, model string) float64) |  |
| `routeTimeProbeTimeout` | const |  |
| `routingHarnessEntryFromMetadata` | func (func routingHarnessEntryFromMetadata(name string, cfg harnesses.HarnessConfig, st harnesses.HarnessStatus) routing.HarnessEntry) |  |
| `routingHarnessUsesAccountBilling` | func (func routingHarnessUsesAccountBilling(entry *routing.HarnessEntry) bool) |  |
| `routingPolicyForName` | func (func routingPolicyForName(cat *modelcatalog.Catalog, name string) string) |  |
| `runAlivenessProbeLoop` | func (func runAlivenessProbeLoop( ctx context.Context, endpoints []alivenessEndpoint, store *routehealth.ProbeStore, prober ProviderAlivenessProber, interval time.Duration, now func() time.Time, sleep func(ctx context.Context, d time.Duration) bool, persistPath string, )) | runAlivenessProbeLoop periodically re-probes each endpoint whose last probe |
| `runQuotaRecoveryProbeLoop` | func (func runQuotaRecoveryProbeLoop( ctx context.Context, store *ProviderQuotaStateStore, probe QuotaRecoveryProber, fallback time.Duration, now func() time.Time, sleep func(ctx context.Context, d time.Duration) bool, )) | runQuotaRecoveryProbeLoop periodically probes providers that the store |
| `runQuotaRecoveryProbePass` | func (func runQuotaRecoveryProbePass( ctx context.Context, store *ProviderQuotaStateStore, probe QuotaRecoveryProber, fallback time.Duration, now func() time.Time, backoffs map[string]time.Duration, ) time.Duration) | runQuotaRecoveryProbePass executes a single sweep over the quota_exhausted |
| `runRouteTimeAlivenessProbes` | func (func runRouteTimeAlivenessProbes( ctx context.Context, endpoints []alivenessEndpoint, store *routehealth.ProbeStore, prober ProviderAlivenessProber, perProbeTimeout time.Duration, )) |  |
| `runStartupAlivenessProbes` | func (func runStartupAlivenessProbes( ctx context.Context, endpoints []alivenessEndpoint, store *routehealth.ProbeStore, prober ProviderAlivenessProber, totalTimeout time.Duration, )) | runStartupAlivenessProbes probes each endpoint sequentially within totalTimeout |
| `scorePowerHintFit` | func (func scorePowerHintFit(power int, policy RoutePowerPolicy) float64) |  |
| `seamOptions` | type (struct) | seamOptions is empty in production builds |
| `service` | type (struct) | service is the concrete FizeauService implementation |
| `serviceConfigToModelSnapshotConfig` | func (func serviceConfigToModelSnapshotConfig(sc ServiceConfig) *modelsnapshot.Config) |  |
| `serviceExecuteWired` | func (func serviceExecuteWired(name string, cfg harnesses.HarnessConfig) bool) |  |
| `serviceImplProviderEntry` | func (func serviceImplProviderEntry(entry ServiceProviderEntry) serviceimpl.ProviderEntry) |  |
| `serviceIsUnreachable` | func (func serviceIsUnreachable(msg string) bool) |  |
| `serviceProviderDefaultInclusion` | func (func serviceProviderDefaultInclusion(entry ServiceProviderEntry) bool) |  |
| `serviceRoutingCatalog` | func (func serviceRoutingCatalog() *modelcatalog.Catalog) |  |
| `serviceRoutingCatalogCandidatesResolver` | func (func serviceRoutingCatalogCandidatesResolver(cat *modelcatalog.Catalog) func(ref, surface string) ([]string, bool)) |  |
| `serviceRoutingCatalogResolver` | func (func serviceRoutingCatalogResolver(cat *modelcatalog.Catalog) func(ref, surface string) (string, bool)) |  |
| `serviceRoutingCatalogSurface` | func (func serviceRoutingCatalogSurface(surface string) (modelcatalog.Surface, bool)) |  |
| `serviceRoutingModelEligibility` | func (func serviceRoutingModelEligibility(entries []routing.HarnessEntry, cat *modelcatalog.Catalog) func(model string) (routing.ModelEligibility, bool)) |  |
| `serviceRoutingReasoningResolver` | func (func serviceRoutingReasoningResolver(cat *modelcatalog.Catalog) func(policy, surface string) (string, bool)) | serviceRoutingReasoningResolver returns the catalog's surface_policy |
| `serviceSessionLog` | type (struct) | serviceSessionLog is the root-facade adapter over the internal session-log |
| `serviceSnapshotCacheRoot` | func (func serviceSnapshotCacheRoot() (string, error)) |  |
| `serviceTrimError` | func (func serviceTrimError(msg string) string) |  |
| `sessionLogPath` | func (func sessionLogPath(sl *serviceSessionLog) string) |  |
| `shortProgressText` | func (func shortProgressText(s string) string) |  |
| `shouldAutoLoadServiceConfig` | func (func shouldAutoLoadServiceConfig(ServiceOptions) bool) |  |
| `shouldEscalateOnError` | func (func shouldEscalateOnError(err error) bool) | shouldEscalateOnError gates ladder escalation to "no eligible candidate" |
| `shouldPreferProviderProbe` | func (func shouldPreferProviderProbe(candidate, current providerProbeResult) bool) |  |
| `snapshotContextWindow` | func (func snapshotContextWindow(pcfg ServiceProviderEntry, cat *modelcatalog.Catalog, modelID string, snapshotWindow int) (int, string)) |  |
| `snapshotEndpointName` | func (func snapshotEndpointName(pcfg ServiceProviderEntry, key snapshotProviderGroupKey) string) |  |
| `snapshotModelIDs` | func (func snapshotModelIDs(rows []modelsnapshot.KnownModel) []string) |  |
| `snapshotProviderContextWindows` | func (func snapshotProviderContextWindows(ctx context.Context, pcfg ServiceProviderEntry, cat *modelcatalog.Catalog, rows []modelsnapshot.KnownModel, discoveredIDs []string) (map[string]int, map[string]string)) |  |
| `snapshotProviderGroupKey` | type (struct) |  |
| `splitEndpointProviderRef` | func (func splitEndpointProviderRef(ref string) (string, string, bool)) |  |
| `staleHarnessRecord` | type (struct) |  |
| `startupProbeTotalTimeout` | const |  |
| `statusErrorType` | func (func statusErrorType(status string) string) |  |
| `statusViewProvider` | func (func statusViewProvider(entry ServiceProviderEntry) statusview.ServiceProvider) |  |
| `stickyRouteAffinityBonus` | const |  |
| `stickyRouteLeaseTTL` | const |  |
| `stringIn` | func (func stringIn(xs []string, v string) bool) |  |
| `subprocessHarnessAutoRoutingModels` | func (func subprocessHarnessAutoRoutingModels(name string, cfg harnesses.HarnessConfig) []string) |  |
| `subprocessHarnessModelIDs` | func (func subprocessHarnessModelIDs(name string, cfg harnesses.HarnessConfig) []string) |  |
| `subprocessProgressState` | type (struct) |  |
| `subscriptionEffectiveCostUSDPer1kTokens` | func (func subscriptionEffectiveCostUSDPer1kTokens(baseCost float64, quotaPercentUsed int, curve SubscriptionCostCurve) float64) |  |
| `subscriptionFallbackPolicy` | func (func subscriptionFallbackPolicy(harnessName string) string) |  |
| `subscriptionQuotaView` | type (alias) |  |
| `summarizeJSONValue` | func (func summarizeJSONValue(raw json.RawMessage) string) |  |
| `summarizeToolInput` | func (func summarizeToolInput(toolName string, input json.RawMessage) string) |  |
| `summarizeToolOutput` | func (func summarizeToolOutput(output string) string) |  |
| `supportedPermissions` | func (func supportedPermissions(cfg harnesses.HarnessConfig) []string) | supportedPermissions extracts the permission levels from PermissionArgs keys, |
| `supportedReasoning` | func (func supportedReasoning(cfg harnesses.HarnessConfig) []string) |  |
| `tcpAlivenessProber` | func (func tcpAlivenessProber(ctx context.Context, _, baseURL string) bool) | tcpAlivenessProber tests endpoint reachability via a TCP connect probe |
| `terminateOwnedProcessGroup` | func (func terminateOwnedProcessGroup(pgid int)) |  |
| `toRoutingQualityOverride` | func (func toRoutingQualityOverride(ov ServiceOverrideData) *routingquality.OverrideData) |  |
| `toTranscriptProgress` | func (func toTranscriptProgress(payload ServiceProgressData) transcript.ProgressPayload) |  |
| `toTranscriptRouteDecision` | func (func toTranscriptRouteDecision(decision RouteDecision) transcript.RouteProgressDecision) |  |
| `toolOutputDetail` | type (alias) |  |
| `toolTaskSummary` | type (alias) |  |
| `toolWiringHookFn` | type (func) |  |
| `validateExplicitHarnessModel` | func (func validateExplicitHarnessModel(name string, cfg harnesses.HarnessConfig, model, provider string) error) |  |
| `validateExplicitHarnessPolicy` | func (func validateExplicitHarnessPolicy(name string, cfg harnesses.HarnessConfig, policy string) error) |  |
| `validateExplicitHarnessQuota` | func (func validateExplicitHarnessQuota(name string, cfg harnesses.HarnessConfig) error) |  |
| `validateExplicitHarnessReasoning` | func (func validateExplicitHarnessReasoning(name string, cfg harnesses.HarnessConfig, value Reasoning) error) |  |
| `validateExplicitProvider` | func (func validateExplicitProvider(sc ServiceConfig, cfg harnesses.HarnessConfig, provider string) error) | validateExplicitProvider rejects pre-dispatch when the caller pinned a |
| `waitForPrimaryQuotaRefreshes` | func (func waitForPrimaryQuotaRefreshes(waits []<-chan struct{}, timeout time.Duration)) |  |
| `withRouteCandidates` | func (func withRouteCandidates(err error, candidates []RouteCandidate) error) |  |
| `wrapExecuteWithHub` | func (func wrapExecuteWithHub(fanout executeEventFanout, sessionID string, outer chan ServiceEvent, ovr *overrideContext, meta map[string]string) chan ServiceEvent) | wrapExecuteWithHub wraps the inner out channel so that every event emitted |
| `writeStaleHarnessRecord` | func (func writeStaleHarnessRecord(path string, record staleHarnessRecord) error) |  |


## Where to look next

- `github.com/easel/fizeau` — the root facade: `FizeauService`, request and
  event types, public errors, and compatibility wrappers that keep embedders
  off `internal/`.
- `internal/serviceimpl/`, `internal/transcript/`, `internal/routehealth/`,
  `internal/quota/`, and `internal/routingquality/` — the concrete runtime
  owners behind the facade. These are useful for repo orientation but remain
  intentionally non-importable.
- `AGENTS.md` — package layout and the rules about what's public vs
  `internal/`; read this before adding new exports.
- `agentcli/` — mountable command tree backed by the public service
  facade. Importable from your own CLI binary if you want fiz subcommands.

## Sub-packages

Most of the implementation lives under `internal/` and is intentionally
not importable. The packages a third-party Go consumer can actually use are:

- `github.com/easel/fizeau` — the root facade (this page).
- `github.com/easel/fizeau/configinit` — blank-import to register the
  default YAML config loader.
- `github.com/easel/fizeau/agentcli` — mountable CLI commands.
- `github.com/easel/fizeau/catalogdist` — model catalog distribution
  helpers.
- `github.com/easel/fizeau/telemetry` — runtime telemetry scaffolding
  for invoke_agent / chat / execute_tool spans.

## Regenerating this page

This page is generated by `cmd/docgen-embedding` from the source code.
Regenerate after any change to the public API:

```bash
make docs-embedding
```

The generator is deterministic — running it twice yields byte-identical
output.
