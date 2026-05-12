package modelregistry

import (
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

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
	Provider   string
	ID         string
	Configured bool

	DiscoveredVia Source
	DiscoveredAt  time.Time

	Family     string
	Version    []int
	Tier       modelcatalog.Tier
	PreRelease bool

	Power           int
	CostInputPerM   float64
	CostOutputPerM  float64
	ContextWindow   int
	ReasoningLevels []string
	QuotaPool       string

	Status          ModelStatus
	AutoRoutable    bool
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
