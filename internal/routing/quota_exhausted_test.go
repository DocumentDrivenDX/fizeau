package routing

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestResolveExcludesQuotaExhaustedProvider(t *testing.T) {
	in := newTestRoutingEngine()
	now := in.Now
	// Pin both providers under the agent harness; mark vidar-omlx exhausted.
	in.ProviderQuotaExhaustedUntil = map[string]time.Time{
		"vidar-omlx": now.Add(5 * time.Minute),
	}
	req := Request{Profile: "cheap", Harness: "agent"}
	dec, err := Resolve(req, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider == "vidar-omlx" {
		t.Fatalf("quota_exhausted vidar-omlx should be excluded; picked it anyway")
	}
	// vidar-omlx must still appear in the trace, marked ineligible with the
	// quota_exhausted filter reason, so callers can render the explanation.
	var found bool
	for _, c := range dec.Candidates {
		if c.Provider == "vidar-omlx" {
			found = true
			if c.Eligible {
				t.Fatalf("vidar-omlx should be ineligible: %#v", c)
			}
			if c.FilterReason != FilterReasonQuotaExhausted {
				t.Fatalf("vidar-omlx FilterReason = %q, want quota_exhausted", c.FilterReason)
			}
		}
	}
	if !found {
		t.Fatalf("vidar-omlx missing from candidate trace")
	}
}

func TestResolveAllProvidersQuotaExhaustedReturnsTypedError(t *testing.T) {
	in := newTestRoutingEngine()
	now := in.Now
	// Restrict to one harness/provider, then exhaust it.
	in.Harnesses = []HarnessEntry{in.Harnesses[0]}
	in.Harnesses[0].Providers = []ProviderEntry{in.Harnesses[0].Providers[0]} // only vidar-omlx
	retryAfter := now.Add(7 * time.Minute)
	in.ProviderQuotaExhaustedUntil = map[string]time.Time{
		"vidar-omlx": retryAfter,
	}

	req := Request{Profile: "cheap", Harness: "agent"}
	_, err := Resolve(req, in)
	if err == nil {
		t.Fatal("expected ErrAllProvidersQuotaExhausted")
	}
	var typed *ErrAllProvidersQuotaExhausted
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As should extract ErrAllProvidersQuotaExhausted: %T %v", err, err)
	}
	if !typed.RetryAfter.Equal(retryAfter) {
		t.Fatalf("RetryAfter = %v, want %v", typed.RetryAfter, retryAfter)
	}
	if !slices.Equal(typed.ExhaustedProviders, []string{"vidar-omlx"}) {
		t.Fatalf("ExhaustedProviders = %v, want [vidar-omlx]", typed.ExhaustedProviders)
	}
	if !errors.Is(err, &ErrAllProvidersQuotaExhausted{}) {
		t.Fatalf("errors.Is should match sentinel")
	}
}

func TestResolveQuotaExhaustedRetryAfterPickedAsEarliest(t *testing.T) {
	in := newTestRoutingEngine()
	now := in.Now
	in.Harnesses = []HarnessEntry{in.Harnesses[0]}
	// Two providers, both exhausted with different retry_afters.
	earliest := now.Add(2 * time.Minute)
	later := now.Add(30 * time.Minute)
	in.ProviderQuotaExhaustedUntil = map[string]time.Time{
		"vidar-omlx": later,
		"openrouter": earliest,
	}
	req := Request{Profile: "cheap", Harness: "agent"}
	_, err := Resolve(req, in)
	var typed *ErrAllProvidersQuotaExhausted
	if !errors.As(err, &typed) {
		t.Fatalf("expected ErrAllProvidersQuotaExhausted, got %T %v", err, err)
	}
	if !typed.RetryAfter.Equal(earliest) {
		t.Fatalf("RetryAfter = %v, want earliest %v", typed.RetryAfter, earliest)
	}
	if len(typed.ExhaustedProviders) != 2 {
		t.Fatalf("ExhaustedProviders = %v, want 2 entries", typed.ExhaustedProviders)
	}
}

func TestResolvePastRetryAfterIsIgnored(t *testing.T) {
	in := newTestRoutingEngine()
	now := in.Now
	in.ProviderQuotaExhaustedUntil = map[string]time.Time{
		"vidar-omlx": now.Add(-time.Minute), // already recovered
	}
	req := Request{Profile: "cheap", Harness: "agent"}
	dec, err := Resolve(req, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// vidar-omlx was past retry; it should still be eligible.
	for _, c := range dec.Candidates {
		if c.Provider == "vidar-omlx" && c.FilterReason == FilterReasonQuotaExhausted {
			t.Fatalf("past retry_after should be ignored: %#v", c)
		}
	}
}

func TestResolveQuotaExhaustedDoesNotFireWhenOtherFailureExists(t *testing.T) {
	in := newTestRoutingEngine()
	now := in.Now
	in.Harnesses = []HarnessEntry{in.Harnesses[0]}
	in.Harnesses[0].Providers = []ProviderEntry{in.Harnesses[0].Providers[0]}
	in.ProviderQuotaExhaustedUntil = map[string]time.Time{
		"vidar-omlx": now.Add(5 * time.Minute),
	}
	// Pin a model the provider does not serve — that's a different rejection
	// (not quota). The quota-exhausted error must not mask the real failure
	// when there is no candidate that would have been eligible save for quota.
	req := Request{Harness: "agent", Provider: "vidar-omlx", Model: "model-not-on-vidar"}
	_, err := Resolve(req, in)
	if err == nil {
		t.Fatal("expected error")
	}
	var quotaErr *ErrAllProvidersQuotaExhausted
	if errors.As(err, &quotaErr) {
		t.Fatalf("ErrAllProvidersQuotaExhausted leaked when the candidate also failed model resolution: %v", err)
	}
}
