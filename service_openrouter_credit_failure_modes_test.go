package fizeau

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/routing"
)

// TestOpenrouterCredits401YieldsCredentialInvalid asserts a 401 from
// /api/v1/credits produces FilterReasonCredentialInvalid (not the
// credential_missing reason that the presence gate emits) and that the
// evidence body names the originating HTTP status so operators can triage
// from routing_decision without grepping logs.
func TestOpenrouterCredits401YieldsCredentialInvalid(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	svc, _, credits := openrouterCreditGateService(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"No auth credentials found"}}`))
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
	dec = openrouterDecisionCandidates(t, dec, err)
	candidate := findOpenRouterCandidate(t, dec)
	if candidate.Eligible {
		t.Fatalf("openrouter candidate eligible with 401 from credits probe: %#v", *candidate)
	}
	if candidate.FilterReason != string(routing.FilterReasonCredentialInvalid) {
		t.Fatalf("openrouter FilterReason=%q, want %q (must be distinct from credential_missing)",
			candidate.FilterReason, routing.FilterReasonCredentialInvalid)
	}
	if !strings.Contains(candidate.Reason, "401") {
		t.Fatalf("openrouter Reason=%q, want originating status code in evidence body", candidate.Reason)
	}
	if got := credits.Load(); got != 1 {
		t.Fatalf("credits probe hits=%d, want exactly 1", got)
	}
}

// TestOpenrouterTransportFailureYieldsProviderUnreachable asserts that a
// transport-level failure (e.g. roundtripper returning net.Error) surfaces
// as FilterReasonProviderUnreachable with eligible=false within the current
// freshness window, and that the evidence body names the transport error
// class.
func TestOpenrouterTransportFailureYieldsProviderUnreachable(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	// Bring up the service with the normal happy-path handler, then swap the
	// store's transport for one that always fails with a net.Error. The
	// service is wired through openrouterCreditGateService so all the other
	// plumbing (config, catalog, snapshot fixture) is in place.
	svc, _, credits := openrouterCreditGateService(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		// This handler must never be hit because the failing transport short
		// circuits client.Do(). Guard the assertion in case the swap regresses.
		t.Fatalf("transport should have failed before handler; got request")
		w.WriteHeader(http.StatusOK)
	})
	svc.openrouterCredit.transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
	dec = openrouterDecisionCandidates(t, dec, err)
	candidate := findOpenRouterCandidate(t, dec)
	if candidate.Eligible {
		t.Fatalf("openrouter candidate eligible after transport failure: %#v", *candidate)
	}
	if candidate.FilterReason != string(routing.FilterReasonProviderUnreachable) {
		t.Fatalf("openrouter FilterReason=%q, want %q",
			candidate.FilterReason, routing.FilterReasonProviderUnreachable)
	}
	if !strings.Contains(candidate.Reason, "transport_error") {
		t.Fatalf("openrouter Reason=%q, want transport error class in evidence body", candidate.Reason)
	}
	if got := credits.Load(); got != 0 {
		t.Fatalf("credits handler hits=%d, want 0 (transport must fail before reaching the server)", got)
	}
}

// TestOpenrouterFivexxClassifiedAsUnreachable guards that a 5xx response is
// classified as provider_unreachable (transient) rather than
// credential_invalid (rotate the key). The originating HTTP status must
// appear in evidence so operators can distinguish e.g. 502 from 503.
func TestOpenrouterFivexxClassifiedAsUnreachable(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	const wantStatus = http.StatusBadGateway

	svc, _, credits := openrouterCreditGateService(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(wantStatus)
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
	dec = openrouterDecisionCandidates(t, dec, err)
	candidate := findOpenRouterCandidate(t, dec)
	if candidate.Eligible {
		t.Fatalf("openrouter candidate eligible with 502 from credits probe: %#v", *candidate)
	}
	if candidate.FilterReason == string(routing.FilterReasonCredentialInvalid) {
		t.Fatalf("openrouter 5xx misclassified as credential_invalid: %#v", *candidate)
	}
	if candidate.FilterReason != string(routing.FilterReasonProviderUnreachable) {
		t.Fatalf("openrouter FilterReason=%q, want %q",
			candidate.FilterReason, routing.FilterReasonProviderUnreachable)
	}
	if !strings.Contains(candidate.Reason, fmt.Sprintf("%d", wantStatus)) {
		t.Fatalf("openrouter Reason=%q, want originating status %d in evidence body",
			candidate.Reason, wantStatus)
	}
	if got := credits.Load(); got != 1 {
		t.Fatalf("credits probe hits=%d, want exactly 1", got)
	}
}

// TestOpenrouterRecoversFromTransientProbeFailure drives the cache through
// one provider_unreachable probe followed by a successful probe on the next
// scheduled pass. After the recovery probe the candidate must return to
// eligible=true with no operator action — fail-open semantics.
func TestOpenrouterRecoversFromTransientProbeFailure(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	const ttl = 30 * time.Second

	// First handler call returns 503; subsequent calls return ample balance.
	var calls atomic.Int32
	svc, _, credits := openrouterCreditGateService(t, func(p *ServiceProviderEntry) {
		p.CreditProbeTTL = ttl
	}, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openrouterCreditFixtureResponse(25.0)))
	})

	// First pass: transient probe failure → provider_unreachable.
	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{})
	dec = openrouterDecisionCandidates(t, dec, err)
	candidate := findOpenRouterCandidate(t, dec)
	if candidate.FilterReason != string(routing.FilterReasonProviderUnreachable) {
		t.Fatalf("openrouter FilterReason=%q after transient failure, want %q",
			candidate.FilterReason, routing.FilterReasonProviderUnreachable)
	}
	if candidate.Eligible {
		t.Fatalf("openrouter candidate eligible during provider_unreachable window: %#v", *candidate)
	}

	// A pass inside the TTL must not re-probe; the unreachable state stays.
	_, _ = svc.ResolveRoute(context.Background(), RouteRequest{})
	if got := credits.Load(); got != 1 {
		t.Fatalf("credits probe hits=%d during freshness window, want 1 (must not re-probe)", got)
	}

	// Drive the fake clock past the TTL by rewinding ObservedAt — the next
	// pass triggers a fresh probe. The successful second response clears
	// the failure record and the candidate returns to eligible=true.
	rec, ok := svc.openrouterCredit.Lookup("openrouter")
	if !ok {
		t.Fatalf("expected cached failure record after first pass")
	}
	rec.ObservedAt = rec.ObservedAt.Add(-2 * ttl)
	svc.openrouterCredit.Record("openrouter", rec)

	dec, err = svc.ResolveRoute(context.Background(), RouteRequest{})
	dec = openrouterDecisionCandidates(t, dec, err)
	candidate = findOpenRouterCandidate(t, dec)
	if !candidate.Eligible {
		t.Fatalf("openrouter candidate still ineligible after recovery probe (FilterReason=%q): %#v",
			candidate.FilterReason, *candidate)
	}
	if candidate.FilterReason == string(routing.FilterReasonProviderUnreachable) {
		t.Fatalf("openrouter still gated as provider_unreachable after success: %#v", *candidate)
	}
	if got := credits.Load(); got != 2 {
		t.Fatalf("credits probe hits=%d after TTL expiry, want 2", got)
	}
}

// roundTripFunc adapts a plain function into a http.RoundTripper. Used by
// the transport-failure test to drive client.Do() down its error path.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
