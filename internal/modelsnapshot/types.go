package modelsnapshot

import (
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

// RefreshMode controls whether Assemble refreshes stale source cache data.
type RefreshMode int

const (
	// RefreshBackground preserves the default stale-while-revalidate behavior:
	// return cached data immediately and refresh stale sources in the background.
	RefreshBackground RefreshMode = iota
	// RefreshNone returns cached data only.
	RefreshNone
	// RefreshForce refreshes sources synchronously before reading them.
	RefreshForce
)

// AssembleOptions controls snapshot assembly behavior.
type AssembleOptions struct {
	Refresh RefreshMode
}

// ProviderEndpoint describes one serving endpoint for providers that can run
// across multiple host:port locations.
type ProviderEndpoint struct {
	Name           string
	BaseURL        string
	ServerInstance string
}

// ProviderConfig describes one named provider configuration.
type ProviderConfig struct {
	Type           string
	BaseURL        string
	ServerInstance string
	Endpoints      []ProviderEndpoint
	APIKey         string
	Headers        map[string]string
	Model          string
	Billing        string

	IncludeByDefault    *bool
	IncludeByDefaultSet bool
	ContextWindow       int
	ConfigError         string
	DailyTokenBudget    int
}

// Config carries the subset of service configuration needed to assemble a
// model snapshot without importing the root package or agentcli.
type Config struct {
	Providers map[string]ProviderConfig
	Default   string
}

// Source records how a model ID was discovered.
type Source string

const (
	SourceNativeAPI  Source = "native_api"
	SourceHarnessPTY Source = "harness_pty"
	SourcePropsAPI   Source = "props_api"
)

// ModelStatus records the provider runtime state relevant to routing.
type ModelStatus string

const (
	StatusAvailable   ModelStatus = "available"
	StatusRateLimited ModelStatus = "rate_limited"
	StatusUnreachable ModelStatus = "unreachable"
	StatusUnknown     ModelStatus = "unknown"
)

// KnownModel is one discovered provider/model identity enriched with catalog
// metadata when the catalog knows the model.
type KnownModel struct {
	Provider     string
	ProviderType string
	Harness      string
	ID           string
	Configured   bool

	EndpointName     string
	EndpointBaseURL  string
	ServerInstance   string
	Billing          modelcatalog.BillingModel
	IncludeByDefault bool

	DiscoveredVia Source
	DiscoveredAt  time.Time

	Family     string
	Version    []int
	Tier       modelcatalog.Tier
	PreRelease bool

	Power            int
	CostInputPerM    float64
	CostOutputPerM   float64
	ContextWindow    int
	ReasoningLevels  []string
	QuotaPool        string
	QuotaRemaining   *int
	RecentP50Latency time.Duration

	Status          ModelStatus
	AutoRoutable    bool
	ExactPinOnly    bool
	ExclusionReason string
}

// ModelSnapshot is the in-memory model registry view assembled from discovery
// cache entries and catalog metadata.
type ModelSnapshot struct {
	Models  []KnownModel
	AsOf    time.Time
	Sources map[string]SourceMeta
}

// SourceMeta summarizes cache state for a discovery source.
type SourceMeta struct {
	LastRefreshedAt time.Time
	Stale           bool
	Error           string
}
