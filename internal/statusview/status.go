package statusview

import (
	"strings"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

// Error describes the most recent normalized status error for a harness,
// provider, or endpoint.
type Error struct {
	Type      string
	Detail    string
	Source    string
	Timestamp time.Time
}

// Account describes authentication/account state without exposing
// provider-specific native files to consumers.
type Account struct {
	Authenticated   bool
	Unauthenticated bool
	Email           string
	PlanType        string
	OrgName         string
	Source          string
	CapturedAt      time.Time
	Fresh           bool
	Detail          string
}

// Endpoint describes one configured provider endpoint probe.
type Endpoint struct {
	Name          string
	BaseURL       string
	ProbeURL      string
	Status        string
	Source        string
	CapturedAt    time.Time
	Fresh         bool
	LastSuccessAt time.Time
	ModelCount    int
	LastError     *Error
}

// Quota is a live quota snapshot for a provider status surface.
type Quota struct {
	Source     string
	Status     string
	CapturedAt time.Time
	LastError  *Error
}

// ServiceProvider carries the subset of provider config needed for status
// normalization without importing the root package.
type ServiceProvider struct {
	Type      string
	BaseURL   string
	Endpoints []ServiceProviderEndpoint
	APIKey    string
}

// ServiceProviderEndpoint carries the subset of endpoint config needed for
// endpoint status normalization.
type ServiceProviderEndpoint struct {
	Name    string
	BaseURL string
}

// ErrorForStatus returns the normalized status error when the provider or
// harness status is not a success value.
func ErrorForStatus(status, source string, capturedAt time.Time) *Error {
	return ErrorForStatusDetail(status, "", source, capturedAt)
}

// ErrorForStatusDetail returns the normalized status error with an explicit
// detail string.
func ErrorForStatusDetail(status, detail, source string, capturedAt time.Time) *Error {
	if status == "" || status == "connected" || status == "ok" {
		return nil
	}
	if detail == "" {
		detail = status
	}
	return &Error{
		Type:      ErrorType(status),
		Detail:    detail,
		Source:    source,
		Timestamp: capturedAt,
	}
}

// ErrorType classifies a provider or harness status string into the public
// error vocabulary.
func ErrorType(status string) string {
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "api_key") || strings.Contains(lower, "unauth") || strings.Contains(lower, "401") || strings.Contains(lower, "403"):
		return "unauthenticated"
	case lower == "unreachable" || strings.Contains(lower, "not found") || strings.Contains(lower, "connection") || strings.Contains(lower, "timeout"):
		return "unavailable"
	default:
		return "error"
	}
}

// QuotaStatus converts freshness plus window evidence into the public quota
// status string used by harness quota projections.
func QuotaStatus(fresh bool, windows []harnesses.QuotaWindow) string {
	if len(windows) == 0 {
		return "unknown"
	}
	for _, w := range windows {
		if w.State == "blocked" || w.State == "exhausted" || w.UsedPercent >= 95 {
			return "blocked"
		}
	}
	if !fresh {
		return "stale"
	}
	return "ok"
}

// AccountFromInfo converts harness account info into the normalized service
// account projection.
func AccountFromInfo(info *harnesses.AccountInfo, source string, capturedAt time.Time, fresh bool) *Account {
	if info == nil {
		return nil
	}
	return &Account{
		Authenticated: true,
		Email:         info.Email,
		PlanType:      info.PlanType,
		OrgName:       info.OrgName,
		Source:        source,
		CapturedAt:    capturedAt,
		Fresh:         fresh,
	}
}

// ProviderAuthStatus projects provider config plus probe status into the
// public account-status surface.
func ProviderAuthStatus(entry ServiceProvider, status string, capturedAt time.Time) Account {
	auth := Account{
		Source:     "service provider config",
		CapturedAt: capturedAt,
		Fresh:      true,
	}
	if ErrorType(status) == "unauthenticated" {
		auth.Unauthenticated = true
		auth.Detail = status
		return auth
	}
	if entry.APIKey != "" {
		auth.Authenticated = true
		return auth
	}
	switch entry.Type {
	case "anthropic", "openrouter":
		auth.Unauthenticated = true
		auth.Detail = "api_key not configured"
	default:
		auth.Detail = "authentication not required or not reported"
	}
	return auth
}

// ProviderEndpointStatus synthesizes endpoint status for providers with
// configured endpoints but no explicit probe breakdown.
func ProviderEndpointStatus(entry ServiceProvider, status string, modelCount int, capturedAt time.Time) []Endpoint {
	source := strings.TrimRight(entry.BaseURL, "/") + "/models"
	if entry.BaseURL == "" {
		source = "service provider config"
	}
	base := Endpoint{
		Name:       "default",
		BaseURL:    entry.BaseURL,
		ProbeURL:   source,
		Status:     EndpointStatusFor(status),
		Source:     source,
		CapturedAt: capturedAt,
		Fresh:      true,
		ModelCount: modelCount,
		LastError:  ErrorForStatus(status, source, capturedAt),
	}
	if base.Status == "connected" {
		base.LastSuccessAt = capturedAt
	}
	out := []Endpoint{base}
	for _, endpoint := range entry.Endpoints {
		out = append(out, Endpoint{
			Name:       endpoint.Name,
			BaseURL:    endpoint.BaseURL,
			ProbeURL:   strings.TrimRight(endpoint.BaseURL, "/") + "/models",
			Status:     "unknown",
			Source:     "service provider config",
			CapturedAt: capturedAt,
			Fresh:      false,
		})
	}
	return out
}

// ProviderQuotaState projects provider config into the provider-status quota
// surface when a provider has a special quota probe endpoint.
func ProviderQuotaState(entry ServiceProvider, capturedAt time.Time) *Quota {
	switch entry.Type {
	case "openrouter":
		source := strings.TrimRight(entry.BaseURL, "/") + "/auth/key"
		if entry.BaseURL == "" {
			source = "openrouter /auth/key"
		}
		if entry.APIKey == "" {
			return &Quota{
				Source:     source,
				Status:     "unauthenticated",
				CapturedAt: capturedAt,
				LastError: &Error{
					Type:      "unauthenticated",
					Detail:    "api_key not configured",
					Source:    source,
					Timestamp: capturedAt,
				},
			}
		}
		return &Quota{
			Source:     source,
			Status:     "unavailable",
			CapturedAt: capturedAt,
			LastError: &Error{
				Type:      "unavailable",
				Detail:    "quota probe not yet captured",
				Source:    source,
				Timestamp: capturedAt,
			},
		}
	default:
		return nil
	}
}

// EndpointStatusFor converts a provider probe status string into the endpoint
// status vocabulary.
func EndpointStatusFor(status string) string {
	if status == "connected" {
		return "connected"
	}
	if ErrorType(status) == "unauthenticated" {
		return "unauthenticated"
	}
	if status == "unreachable" || ErrorType(status) == "unavailable" {
		return "unreachable"
	}
	return "error"
}
