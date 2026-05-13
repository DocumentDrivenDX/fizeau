package routing

import (
	"errors"
	"strings"
	"testing"

	"github.com/easel/fizeau/internal/modelcatalog"
)

// excludedProviderInputs returns a minimal Inputs with two harnesses: "fiz"
// hosting an opt-out "payg" provider (ExcludeFromDefaultRouting=true) and
// "claude" as a default-eligible subscription harness.
func excludedProviderInputs() Inputs {
	return Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				Surface:             "embedded-openai",
				CostClass:           "medium",
				IsHTTPProvider:      true,
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{
						Name:                      "payg",
						DefaultModel:              "gpt-4o",
						Billing:                   modelcatalog.BillingModelPerToken,
						ActualCashSpend:           true,
						ExcludeFromDefaultRouting: true,
					},
				},
			},
			{
				Name:                "claude",
				Surface:             "claude",
				CostClass:           "medium",
				IsSubscription:      true,
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				QuotaOK:             true,
				SubscriptionOK:      true,
				SupportedModels:     []string{"claude-sonnet-4-6"},
				DefaultModel:        "claude-sonnet-4-6",
			},
		},
	}
}

// TestIncludeByDefaultFalseExcludesProviderFromUnpinnedRouting verifies that a
// pay-per-token provider with ExcludeFromDefaultRouting=true is absent from
// default routing candidates when the request does not pin a provider.
func TestIncludeByDefaultFalseExcludesProviderFromUnpinnedRouting(t *testing.T) {
	in := excludedProviderInputs()
	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider == "payg" {
		t.Fatal("payg (ExcludeFromDefaultRouting=true) must not be selected for unpinned request")
	}
	var paygCandidate *Candidate
	for i := range dec.Candidates {
		if dec.Candidates[i].Provider == "payg" {
			paygCandidate = &dec.Candidates[i]
			break
		}
	}
	if paygCandidate == nil {
		t.Fatal("payg candidate not found in decision")
	}
	if paygCandidate.Eligible {
		t.Fatalf("payg candidate.Eligible=true, want false for excluded-from-default provider")
	}
	if paygCandidate.FilterReason != FilterReasonMeteredOptInRequired {
		t.Fatalf("payg FilterReason=%q, want %q", paygCandidate.FilterReason, FilterReasonMeteredOptInRequired)
	}
	if !strings.Contains(paygCandidate.Reason, "metered opt-in") {
		t.Fatalf("payg Reason=%q, want it to mention metered opt-in", paygCandidate.Reason)
	}
}

// TestIncludeByDefaultFalseForFixedProviderUsesDefaultExclusionReason verifies
// that non-metered providers still surface the default-routing exclusion gate.
func TestIncludeByDefaultFalseForFixedProviderUsesDefaultExclusionReason(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				Surface:             "embedded-openai",
				CostClass:           "local",
				IsLocal:             true,
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{
						Name:                      "local-optout",
						DefaultModel:              "local-good",
						Billing:                   modelcatalog.BillingModelFixed,
						ExcludeFromDefaultRouting: true,
						SupportsTools:             true,
					},
					{
						Name:          "local",
						DefaultModel:  "local-good",
						Billing:       modelcatalog.BillingModelFixed,
						SupportsTools: true,
					},
				},
			},
		},
	}

	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	candidate, ok := candidateByProvider(dec.Candidates, "local-optout")
	if !ok {
		t.Fatal("local-optout candidate not found")
	}
	if candidate.Eligible {
		t.Fatalf("local-optout candidate should be rejected: %#v", candidate)
	}
	if candidate.FilterReason != FilterReasonProviderExcludedFromDefault {
		t.Fatalf("local-optout FilterReason=%q, want %q", candidate.FilterReason, FilterReasonProviderExcludedFromDefault)
	}
}

// TestIncludeByDefaultFalseBypassedByExplicitProviderPin verifies that an
// explicit provider pin reaches an ExcludeFromDefaultRouting=true provider.
func TestIncludeByDefaultFalseBypassedByExplicitProviderPin(t *testing.T) {
	in := excludedProviderInputs()
	dec, err := Resolve(Request{Provider: "payg"}, in)
	if err != nil {
		t.Fatalf("Resolve with explicit provider pin: %v", err)
	}
	if dec.Provider != "payg" {
		t.Fatalf("Provider=%q, want payg when explicitly pinned", dec.Provider)
	}
}

// TestIncludeByDefaultFalseBypassedByExactModelPin verifies that an explicit
// model pin reaches an ExcludeFromDefaultRouting=true provider.
func TestIncludeByDefaultFalseBypassedByExactModelPin(t *testing.T) {
	in := excludedProviderInputs()
	dec, err := Resolve(Request{Model: "gpt-4o"}, in)
	if err != nil {
		t.Fatalf("Resolve with explicit model pin: %v", err)
	}
	if dec.Provider != "payg" {
		t.Fatalf("Provider=%q, want payg when exact model pinned", dec.Provider)
	}
	if dec.Model != "gpt-4o" {
		t.Fatalf("Model=%q, want gpt-4o when exact model pinned", dec.Model)
	}
}

// TestIncludeByDefaultTrueUnchangedBehavior verifies that a provider without
// ExcludeFromDefaultRouting set (zero value = false = include) is selected
// normally for unpinned requests.
func TestIncludeByDefaultTrueUnchangedBehavior(t *testing.T) {
	in := Inputs{
		Harnesses: []HarnessEntry{
			{
				Name:                "fiz",
				Surface:             "embedded-openai",
				CostClass:           "local",
				IsLocal:             true,
				AutoRoutingEligible: true,
				Available:           true,
				ExactPinSupport:     true,
				SupportsTools:       true,
				Providers: []ProviderEntry{
					{
						Name:                      "local",
						DefaultModel:              "llama3",
						ExcludeFromDefaultRouting: false, // explicitly false = include
					},
				},
			},
		},
	}
	dec, err := Resolve(Request{Policy: "default"}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "local" {
		t.Fatalf("Provider=%q, want local for default-included provider", dec.Provider)
	}
}

func TestCheckPowerEligibilityKnownModelSnapshotCatalogOnly(t *testing.T) {
	lookup := func(model string) (ModelEligibility, bool) {
		switch model {
		case "catalog-only-model":
			return ModelEligibility{Power: 5, ExactPinOnly: true, AutoRoutable: false}, true
		case "gpt-5.5":
			return ModelEligibility{Power: 10, AutoRoutable: true}, true
		default:
			return ModelEligibility{}, false
		}
	}

	if got, fr := CheckPowerEligibility(lookup, "catalog-only-model", Request{}); got == "" || fr != FilterReasonExactPinOnly {
		t.Fatalf("CheckPowerEligibility(catalog-only-model) = (%q, %q), want exact-pin-only rejection", got, fr)
	}
	if got, fr := CheckPowerEligibility(lookup, "gpt-5.5", Request{}); got != "" || fr != FilterReasonEligible {
		t.Fatalf("CheckPowerEligibility(gpt-5.5) = (%q, %q), want eligible", got, fr)
	}
}

func TestCheckPowerEligibilityKnownModelSnapshotHardPinBypassesCatalogOnlyGate(t *testing.T) {
	lookup := func(model string) (ModelEligibility, bool) {
		switch model {
		case "catalog-only-model":
			return ModelEligibility{Power: 5, ExactPinOnly: true, AutoRoutable: false}, true
		default:
			return ModelEligibility{}, false
		}
	}

	if got, fr := CheckPowerEligibility(lookup, "catalog-only-model", Request{Model: "catalog-only-model"}); got != "" || fr != FilterReasonEligible {
		t.Fatalf("CheckPowerEligibility(hard pin) = (%q, %q), want eligible bypass", got, fr)
	}
}

func TestPinPinConflictHarnessIncompatibleWithModel(t *testing.T) {
	in := Inputs{Harnesses: []HarnessEntry{{
		Name:                "claude",
		Surface:             "claude",
		CostClass:           "medium",
		IsSubscription:      true,
		AutoRoutingEligible: true,
		Available:           true,
		ExactPinSupport:     true,
		SupportedModels:     []string{"opus-4.7"},
		SupportsTools:       true,
	}}}

	_, err := Resolve(Request{Harness: "claude", Model: "qwen3.6"}, in)
	if err == nil {
		t.Fatal("expected harness/model pin conflict")
	}
	var typed *ErrUnsatisfiablePin
	if !errors.As(err, &typed) {
		t.Fatalf("errors.As ErrUnsatisfiablePin: %T %v", err, err)
	}
	if typed.Pin != "harness=claude+model=qwen3.6" {
		t.Fatalf("Pin=%q, want harness=claude+model=qwen3.6", typed.Pin)
	}
}
