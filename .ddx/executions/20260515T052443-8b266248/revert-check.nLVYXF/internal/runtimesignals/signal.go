// Package runtimesignals collects volatile per-provider runtime signals
// (status, quota remaining, recent p50 latency) and writes them to the
// runtime tier of M1's per-source on-disk cache (ADR-012). M2's snapshot
// assembler reads the cache files to populate KnownModel.Status,
// .QuotaRemaining, and .RecentP50Latency.
package runtimesignals

import "time"

// ModelStatus represents the operational availability of a provider.
type ModelStatus string

const (
	StatusAvailable ModelStatus = "available"
	StatusDegraded  ModelStatus = "degraded"
	StatusExhausted ModelStatus = "exhausted"
	StatusUnknown   ModelStatus = "unknown"
)

// Signal is the runtime state snapshot for one provider. It is serialized as
// JSON to runtime/<provider>.json in M1's cache directory.
type Signal struct {
	Provider         string        `json:"provider"`
	Status           ModelStatus   `json:"status"`
	QuotaRemaining   *int          `json:"quota_remaining,omitempty"`
	QuotaResetAt     *time.Time    `json:"quota_reset_at,omitempty"`
	RecentP50Latency time.Duration `json:"recent_p50_latency_ns"`
	LastSuccessAt    *time.Time    `json:"last_success_at,omitempty"`
	LastErrorAt      *time.Time    `json:"last_error_at,omitempty"`
	LastErrorMsg     string        `json:"last_error_msg,omitempty"`
	RecordedAt       time.Time     `json:"recorded_at"`
}
