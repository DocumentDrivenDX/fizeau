package agent

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/agent/internal/harnesses"
	"github.com/DocumentDrivenDX/agent/internal/routing"
)

func TestRouteCandidateFromInternalMapsFields(t *testing.T) {
	candidate := routing.Candidate{
		Harness:  "agent",
		Provider: "local",
		Endpoint: "primary",
		Model:    "model-a",
		Score:    42.5,
		Eligible: true,
		Reason:   "profile=cheap; score=42.5",
	}

	got := routeCandidateFromInternal(candidate)
	if got.Harness != candidate.Harness ||
		got.Provider != candidate.Provider ||
		got.Endpoint != candidate.Endpoint ||
		got.Model != candidate.Model ||
		got.Score != candidate.Score ||
		got.Eligible != candidate.Eligible {
		t.Fatalf("routeCandidateFromInternal()=%#v, want fields from %#v", got, candidate)
	}
	if got.Reason != candidate.Reason {
		t.Fatalf("eligible Reason=%q, want %q", got.Reason, candidate.Reason)
	}

	rejected := candidate
	rejected.Eligible = false
	rejected.Reason = "model not in harness allow-list"
	got = routeCandidateFromInternal(rejected)
	if got.Reason != rejected.Reason {
		t.Fatalf("rejected Reason=%q, want %q", got.Reason, rejected.Reason)
	}
}

func TestResolveRouteSuccessIncludesCandidates(t *testing.T) {
	svc := publicRouteTraceService(&fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", Model: "model-a"},
		},
		names:       []string{"local"},
		defaultName: "local",
	})

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness: "agent",
		Model:   "model-a",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil {
		t.Fatal("ResolveRoute returned nil decision")
	}
	if dec.Harness != "agent" || dec.Provider != "local" || dec.Model != "model-a" {
		t.Fatalf("decision=%#v, want agent/local/model-a", dec)
	}
	if len(dec.Candidates) != 1 {
		t.Fatalf("Candidates length=%d, want 1: %#v", len(dec.Candidates), dec.Candidates)
	}
	candidate := dec.Candidates[0]
	if !candidate.Eligible || candidate.Harness != "agent" || candidate.Provider != "local" || candidate.Model != "model-a" {
		t.Fatalf("candidate=%#v, want eligible agent/local/model-a", candidate)
	}
	if !strings.Contains(candidate.Reason, "score=") {
		t.Fatalf("eligible candidate Reason=%q, want scoring reason", candidate.Reason)
	}
}

func TestResolveRouteErrorIncludesCandidatesAndTraceError(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "redacted")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "")
	t.Setenv("GOOGLE_GENAI_USE_GCA", "")
	t.Setenv("GEMINI_CLI_USE_COMPUTE_ADC", "")
	t.Setenv("CLOUD_SHELL", "")

	registry := harnesses.NewRegistry()
	registry.LookPath = func(file string) (string, error) {
		if file == "gemini" {
			return "/usr/bin/gemini", nil
		}
		return "", os.ErrNotExist
	}
	svc := &service{
		opts:     ServiceOptions{},
		registry: registry,
		hub:      newSessionHub(),
	}

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Model: "minimax/minimax-m2.7",
	})
	if err == nil {
		t.Fatal("ResolveRoute expected no viable candidate error")
	}
	if dec == nil {
		t.Fatal("ResolveRoute error path returned nil decision")
	}
	if dec.Harness != "" || dec.Provider != "" || dec.Model != "" {
		t.Fatalf("error decision selected a candidate: %#v", dec)
	}
	if len(dec.Candidates) == 0 {
		t.Fatal("error decision Candidates is empty")
	}

	var noViable *routing.NoViableCandidateError
	if !errors.As(err, &noViable) {
		t.Fatalf("errors.As no viable: %T %v", err, err)
	}
	var traced DecisionWithCandidates
	if !errors.As(err, &traced) {
		t.Fatalf("errors.As DecisionWithCandidates: %T %v", err, err)
	}
	tracedCandidates := traced.RouteCandidates()
	if len(tracedCandidates) != len(dec.Candidates) {
		t.Fatalf("traced candidates length=%d, decision candidates length=%d", len(tracedCandidates), len(dec.Candidates))
	}
	tracedCandidates[0].Reason = "mutated"
	if dec.Candidates[0].Reason == "mutated" {
		t.Fatal("RouteCandidates returned an alias of the decision candidates")
	}

	var sawGeminiRejection bool
	for _, candidate := range dec.Candidates {
		if candidate.Harness == "gemini" {
			sawGeminiRejection = true
			if candidate.Eligible {
				t.Fatalf("gemini candidate should be rejected: %#v", candidate)
			}
			if candidate.Reason != "model not in harness allow-list" {
				t.Fatalf("gemini rejection reason=%q, want allow-list reason", candidate.Reason)
			}
		}
	}
	if !sawGeminiRejection {
		t.Fatalf("no gemini rejection in candidates: %#v", dec.Candidates)
	}
}

func publicRouteTraceService(sc ServiceConfig) *service {
	return &service{
		opts:     ServiceOptions{ServiceConfig: sc},
		registry: harnesses.NewRegistry(),
		hub:      newSessionHub(),
	}
}

func TestDecisionWithCandidatesCopiesInput(t *testing.T) {
	candidates := []RouteCandidate{{Harness: "agent", Reason: "original"}}
	err := withRouteCandidates(errors.New("no viable routing candidate"), candidates)

	candidates[0].Reason = "changed"
	var traced DecisionWithCandidates
	if !errors.As(err, &traced) {
		t.Fatalf("errors.As DecisionWithCandidates: %T %v", err, err)
	}
	got := traced.RouteCandidates()
	if len(got) != 1 || got[0].Reason != "original" {
		t.Fatalf("RouteCandidates=%#v, want copied original candidate", got)
	}
}
