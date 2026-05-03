package fizeau

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

func TestNoViableProviderForNowExposesFields(t *testing.T) {
	retry := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)
	src := &routing.ErrAllProvidersQuotaExhausted{
		RetryAfter:         retry,
		ExhaustedProviders: []string{"openai", "openrouter"},
	}
	mapped := publicRoutingError(src, nil)

	var typed *NoViableProviderForNow
	if !errors.As(mapped, &typed) {
		t.Fatalf("errors.As should extract NoViableProviderForNow: %T %v", mapped, mapped)
	}
	if !typed.RetryAfter.Equal(retry) {
		t.Fatalf("RetryAfter = %v, want %v", typed.RetryAfter, retry)
	}
	if !slices.Equal(typed.ExhaustedProviders, []string{"openai", "openrouter"}) {
		t.Fatalf("ExhaustedProviders = %v, want [openai openrouter]", typed.ExhaustedProviders)
	}

	if !errors.Is(mapped, &NoViableProviderForNow{}) {
		t.Fatalf("errors.Is should match the typed sentinel")
	}

	// Mutating the public-error slice must not write back into the routing
	// internal error (caller-isolated copy).
	typed.ExhaustedProviders[0] = "mutated"
	if src.ExhaustedProviders[0] != "openai" {
		t.Fatalf("publicRoutingError must copy ExhaustedProviders; saw mutation back: %v", src.ExhaustedProviders)
	}
}

func TestNoViableProviderForNowDistinctFromOtherErrors(t *testing.T) {
	err := &NoViableProviderForNow{RetryAfter: time.Now().Add(time.Minute)}
	if errors.Is(err, ErrUnknownProvider{}) {
		t.Fatal("NoViableProviderForNow should not match ErrUnknownProvider")
	}
	if errors.Is(err, ErrNoLiveProvider{}) {
		t.Fatal("NoViableProviderForNow should not match ErrNoLiveProvider")
	}
	if errors.Is(err, ErrNoProfileCandidate{}) {
		t.Fatal("NoViableProviderForNow should not match ErrNoProfileCandidate")
	}
}
