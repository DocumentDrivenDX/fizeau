package fizeau

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/DocumentDrivenDX/fizeau/internal/routing"
)

// TestServiceExecuteRequestNoPreResolved is an AST guard against the
// removed PreResolved field. ADR-005 step 1 deleted it; reintroducing it
// would silently re-enable caller-supplied route decisions.
func TestServiceExecuteRequestNoPreResolved(t *testing.T) {
	requireStructHasNoField(t, "service.go", "ServiceExecuteRequest", "PreResolved")
	requireStructHasNoField(t, "service.go", "RouteRequest", "PreResolved")
}

// TestServiceExecuteRequestHasAutoSelectionFields is an AST guard for the
// EstimatedPromptTokens / RequiresTools fields that ADR-005 step 1 added
// to ServiceExecuteRequest and RouteRequest.
func TestServiceExecuteRequestHasAutoSelectionFields(t *testing.T) {
	requireStructHasField(t, "service.go", "ServiceExecuteRequest", "EstimatedPromptTokens")
	requireStructHasField(t, "service.go", "ServiceExecuteRequest", "RequiresTools")
	requireStructHasField(t, "service.go", "RouteRequest", "EstimatedPromptTokens")
	requireStructHasField(t, "service.go", "RouteRequest", "RequiresTools")
}

func requireStructHasField(t *testing.T, file, structName, field string) {
	t.Helper()
	if structHasField(t, file, structName, field) {
		return
	}
	t.Fatalf("expected struct %s in %s to declare field %s", structName, file, field)
}

func requireStructHasNoField(t *testing.T, file, structName, field string) {
	t.Helper()
	if !structHasField(t, file, structName, field) {
		return
	}
	t.Fatalf("struct %s in %s must not declare field %s (ADR-005 step 1)", structName, file, field)
}

func structHasField(t *testing.T, file, structName, field string) bool {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var found bool
	ast.Inspect(parsed, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != structName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			for _, name := range f.Names {
				if name.Name == field {
					found = true
					return false
				}
			}
		}
		return false
	})
	return found
}

// TestRouteCandidateExposesComponentScores verifies that ResolveRoute
// returns candidates whose component scores are populated from the
// internal routing engine's per-axis signals.
func TestRouteCandidateExposesComponentScores(t *testing.T) {
	cand := routing.Candidate{
		Harness:            "fiz",
		Provider:           "local",
		Model:              "model-a",
		Score:              42,
		CostUSDPer1kTokens: 0.012,
		CostSource:         routing.CostSourceCatalog,
		Eligible:           true,
		Reason:             "profile=cheap; score=42",
		LatencyMS:          150,
		SuccessRate:        0.95,
		CostClass:          "cheap",
	}
	got := routeCandidateFromInternal(cand)
	if got.Components.Cost != 0.012 {
		t.Errorf("Components.Cost=%v, want 0.012", got.Components.Cost)
	}
	if got.Components.LatencyMS != 150 {
		t.Errorf("Components.LatencyMS=%v, want 150", got.Components.LatencyMS)
	}
	if got.Components.SuccessRate != 0.95 {
		t.Errorf("Components.SuccessRate=%v, want 0.95", got.Components.SuccessRate)
	}
	if got.Components.Capability == 0 {
		t.Errorf("Components.Capability=0; want non-zero for cheap class")
	}
	if got.FilterReason != "" {
		t.Errorf("eligible candidate FilterReason=%q, want empty", got.FilterReason)
	}
}

// TestRouteCandidateFilterReasonClassification verifies the public
// FilterReason enum surfaces canonical values for ineligible candidates.
// The mapping is a typed passthrough: the internal engine sets
// routing.FilterReason at the rejection site, and routeCandidateFromInternal
// forwards it to the public string surface without parsing the free-form
// Reason text (agent-2c55b8a4).
func TestRouteCandidateFilterReasonClassification(t *testing.T) {
	cases := []struct {
		name     string
		typed    routing.FilterReason
		want     string
		freeform string // arbitrary text — must not influence classification
	}{
		{"context too small", routing.FilterReasonContextTooSmall, FilterReasonContextTooSmall, "ctx window 4096 < required 8000"},
		{"no tool support", routing.FilterReasonNoToolSupport, FilterReasonNoToolSupport, "tools unavailable"},
		{"reasoning unsupported", routing.FilterReasonReasoningUnsupported, FilterReasonReasoningUnsupported, "thinking high not advertised"},
		{"unhealthy", routing.FilterReasonUnhealthy, FilterReasonUnhealthy, "harness offline"},
		{"scored below top", routing.FilterReasonScoredBelowTop, FilterReasonScoredBelowTop, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := routeCandidateFromInternal(routing.Candidate{
				Eligible:     false,
				Reason:       tc.freeform,
				FilterReason: tc.typed,
			})
			if got.FilterReason != tc.want {
				t.Errorf("typed=%q (Reason=%q): FilterReason=%q, want %q", tc.typed, tc.freeform, got.FilterReason, tc.want)
			}
		})
	}
}

// TestResolveRouteIsInformationalOnly verifies that ResolveRoute returns
// ranked candidates through the engine flow.
func TestResolveRouteIsInformationalOnly(t *testing.T) {
	sc := &fakeServiceConfig{
		providers: map[string]ServiceProviderEntry{
			"local": {Type: "test", BaseURL: "http://127.0.0.1:9999/v1", Model: "model-a"},
		},
		names:       []string{"local"},
		defaultName: "local",
	}
	svc := publicRouteTraceService(sc)

	dec, err := svc.ResolveRoute(context.Background(), RouteRequest{
		Harness: "fiz",
		Model:   "model-a",
	})
	if err != nil {
		t.Fatalf("ResolveRoute: %v", err)
	}
	if dec == nil || len(dec.Candidates) == 0 {
		t.Fatalf("expected engine-flow decision, got %#v", dec)
	}
}

// TestRoutingDecisionEventComponentsCarriesPerCandidateScores verifies
// the routing-decision event payload includes per-candidate component
// scores and filter_reason fields (ADR-005 AC#5).
func TestRoutingDecisionEventComponentsCarriesPerCandidateScores(t *testing.T) {
	candidates := []RouteCandidate{
		{
			Harness:            "fiz",
			Provider:           "alpha",
			Model:              "alpha-1",
			Score:              80,
			CostUSDPer1kTokens: 0.002,
			CostSource:         routing.CostSourceCatalog,
			Eligible:           true,
			Reason:             "profile=cheap; score=80.0",
			Components: RouteCandidateComponents{
				Power:            7,
				Cost:             0.002,
				CostClass:        "local",
				LatencyMS:        120,
				SpeedTPS:         40,
				SuccessRate:      0.9,
				QuotaOK:          true,
				QuotaPercentUsed: 20,
				QuotaTrend:       routing.QuotaTrendHealthy,
				Capability:       1,
			},
		},
		{
			Harness:      "fiz",
			Provider:     "beta",
			Model:        "beta-1",
			Eligible:     false,
			Reason:       "context window 4096 < required 8000",
			FilterReason: FilterReasonContextTooSmall,
		},
	}
	out := routingDecisionEventCandidates(candidates)
	if len(out) != 2 {
		t.Fatalf("len(out)=%d, want 2", len(out))
	}
	if out[0].Components.LatencyMS != 120 {
		t.Errorf("first candidate LatencyMS=%v, want 120", out[0].Components.LatencyMS)
	}
	if out[0].Components.SuccessRate != 0.9 {
		t.Errorf("first candidate SuccessRate=%v, want 0.9", out[0].Components.SuccessRate)
	}
	if out[0].Components.Power != 7 || out[0].Components.SpeedTPS != 40 || out[0].Components.QuotaTrend != routing.QuotaTrendHealthy {
		t.Errorf("first candidate Components=%#v, want power/speed/quota carried through", out[0].Components)
	}
	if out[0].FilterReason != "" {
		t.Errorf("eligible candidate event FilterReason=%q, want empty", out[0].FilterReason)
	}
	if out[1].FilterReason != FilterReasonContextTooSmall {
		t.Errorf("rejected candidate event FilterReason=%q, want %q", out[1].FilterReason, FilterReasonContextTooSmall)
	}
}

// TestRouteRequestMinContextWindowDerivedFromEstimatedTokens verifies
// the public-to-internal mapping for the auto-selection inputs that
// ADR-005 step 1 added: EstimatedPromptTokens populates routing.Request
// such that MinContextWindow() returns a positive value, which is the
// hook the engine's context-window filter consults in step 2
// (agent-d9c358ba). End-to-end FilterReason assertions through
// ResolveRoute live in step 2's tests; step 1's responsibility ends at
// the public-to-internal boundary.
func TestRouteRequestMinContextWindowDerivedFromEstimatedTokens(t *testing.T) {
	rReq := routing.Request{
		EstimatedPromptTokens: 1_000_000,
		RequiresTools:         true,
	}
	if rReq.MinContextWindow() == 0 {
		t.Fatal("MinContextWindow must be positive when EstimatedPromptTokens is set")
	}
}
