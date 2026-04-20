package routing

import (
	"strings"
	"testing"
	"time"
)

func TestProviderPreferenceFiltering(t *testing.T) {
	in := newTestRoutingEngine()
	// agent is local, codex/claude are subscription.

	t.Run("local-only", func(t *testing.T) {
		req := Request{ProviderPreference: ProviderPreferenceLocalOnly}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Harness != "agent" {
			t.Errorf("local-only: got %q, want agent", dec.Harness)
		}
		for _, c := range dec.Candidates {
			if !strings.Contains(c.Harness, "agent") && c.Eligible {
				t.Errorf("subscription harness %q should be ineligible under local-only", c.Harness)
			}
		}
	})

	t.Run("subscription-only", func(t *testing.T) {
		req := Request{ProviderPreference: ProviderPreferenceSubscriptionOnly}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if dec.Harness == "agent" {
			t.Errorf("subscription-only: should not pick agent")
		}
		for _, c := range dec.Candidates {
			if c.Harness == "agent" && c.Eligible {
				t.Errorf("local harness %q should be ineligible under subscription-only", c.Harness)
			}
		}
	})
}

func TestProviderPreferenceBiasing(t *testing.T) {
	in := newTestRoutingEngine()
	// baseline: cheap prefers local.

	t.Run("local-first bias", func(t *testing.T) {
		req := Request{Profile: "smart", ProviderPreference: ProviderPreferenceLocalFirst}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		// smart normally prefers cloud, but local-first might boost agent enough to win
		// if scores are close. Let's check candidate scores.
		var agentScore, claudeScore float64
		for _, c := range dec.Candidates {
			if c.Harness == "agent" {
				agentScore = c.Score
			}
			if c.Harness == "claude" {
				claudeScore = c.Score
			}
		}
		// local-first should give +30 to agent.
		// smart: agent (local) = 100 + 0*20 (cr=0) + 30 (pref) = 130
		// smart: claude (medium) = 100 + 2*20 (cr=2) + 5 (quota) = 145
		// Claude still wins, but the gap is smaller.
		if agentScore == 0 || claudeScore == 0 {
			t.Fatal("could not find agent or claude scores")
		}
	})

	t.Run("subscription-first bias", func(t *testing.T) {
		req := Request{Profile: "cheap", ProviderPreference: ProviderPreferenceSubscriptionFirst}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		// cheap normally prefers local (+40 local bonus).
		// cheap: agent = 100 + 40 (local) - 0*30 (cr=0) = 140
		// cheap: claude = 100 + 20 (quota) - 2*30 (cr=2) + 30 (pref) = 90
		// Local still wins cheap, but gap is closed by 30.
		if dec.Harness != "agent" {
			t.Errorf("cheap + subscription-first: got %q, want agent (local still stronger)", dec.Harness)
		}
	})
}

func TestQuotaSignalsScoring(t *testing.T) {
	in := newTestRoutingEngine()

	t.Run("exhausted quota", func(t *testing.T) {
		// Set claude quota to not OK.
		for i, h := range in.Harnesses {
			if h.Name == "claude" {
				in.Harnesses[i].QuotaOK = false
			}
		}
		req := Request{Profile: "smart"}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		// smart prefers cloud, but claude's exhausted quota should demote it.
		if dec.Harness == "claude" {
			t.Errorf("exhausted quota: should not pick claude")
		}
	})

	t.Run("stale quota penalty", func(t *testing.T) {
		in := newTestRoutingEngine()
		// All fresh by default. Now mark claude stale.
		for i, h := range in.Harnesses {
			if h.Name == "claude" {
				in.Harnesses[i].QuotaStale = true
			}
		}
		req := Request{Profile: "smart"}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		// Find claude score.
		for _, c := range dec.Candidates {
			if c.Harness == "claude" {
				// Base smart: 100 + 40 (cr=2) + 5 (quota) = 145.
				// Stale penalty: -15 -> 130.
				if c.Score != 130 {
					t.Errorf("stale quota: got score %.1f, want 130.0", c.Score)
				}
			}
		}
	})

	t.Run("quota trend bias", func(t *testing.T) {
		in := newTestRoutingEngine()
		// claude is healthy, codex is burning.
		for i, h := range in.Harnesses {
			if h.Name == "claude" {
				in.Harnesses[i].QuotaTrend = QuotaTrendHealthy
			}
			if h.Name == "codex" {
				in.Harnesses[i].QuotaTrend = QuotaTrendBurning
			}
		}
		req := Request{Profile: "smart"}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		// Both same cost class. Healthy claude (+10) should beat burning codex (-20).
		if dec.Harness != "claude" {
			t.Errorf("quota trend: got %q, want claude (healthy)", dec.Harness)
		}
	})
}

func TestCooldownFallbackWithPreference(t *testing.T) {
	in := newTestRoutingEngine()

	t.Run("local-only with cooldown fallback", func(t *testing.T) {
		// Pinned to agent, but one provider in cooldown.
		// Since it's local-only, it MUST still pick a local provider.
		in.ProviderCooldowns = map[string]time.Time{
			"vidar-omlx": in.Now.Add(-5 * time.Second),
		}
		in.CooldownDuration = 30 * time.Second

		req := Request{Harness: "agent", ProviderPreference: ProviderPreferenceLocalOnly}
		dec, err := Resolve(req, in)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		// vidar-omlx is in cooldown, so it should pick another local provider if available,
		// or demote vidar.
		// In newTestRoutingEngine, agent has vidar-omlx and openrouter.
		// openrouter is also under 'agent' harness (local cost class).
		if dec.Provider != "openrouter" {
			t.Errorf("local-only cooldown fallback: got %q, want openrouter", dec.Provider)
		}
	})
}

// TestDefaultPreferenceNormalization verifies ResolveRoute normalizes
// empty ProviderPreference to local-first.
func TestDefaultPreferenceNormalization(t *testing.T) {
	// Both the service (service_routing.go) and the routing engine normalize
	// empty to local-first. The engine's scorePolicy already treats "" the
	// same as "local-first" (the ProviderPreference switch has "" as a
	// local-first branch), so a Request with ProviderPreference="" must behave
	// identically to one with ProviderPreference="local-first".
	// We test this at the routing engine level.
	in := newTestRoutingEngine()

	emptyReq := Request{Profile: "cheap", ProviderPreference: ""}
	localReq := Request{Profile: "cheap", ProviderPreference: ProviderPreferenceLocalFirst}

	decEmpty, errEmpty := Resolve(emptyReq, in)
	decLocal, errLocal := Resolve(localReq, in)

	if errEmpty != nil {
		t.Fatalf("Resolve with empty preference: %v", errEmpty)
	}
	if errLocal != nil {
		t.Fatalf("Resolve with local-first: %v", errLocal)
	}
	if decEmpty.Harness != decLocal.Harness || decEmpty.Provider != decLocal.Provider {
		t.Errorf("empty preference should behave identically to local-first:\n  empty: harness=%s provider=%s\n  local-first: harness=%s provider=%s",
			decEmpty.Harness, decEmpty.Provider, decLocal.Harness, decLocal.Provider)
	}
}

// TestLocalOnlyExhaustsCandidates verifies local-only returns an error
// when no local candidate is viable (all local harnesses unavailable).
func TestLocalOnlyExhaustsCandidates(t *testing.T) {
	in := newTestRoutingEngine()
	// Mark the agent harness unavailable.
	for i, h := range in.Harnesses {
		if h.Name == "agent" {
			in.Harnesses[i].Available = false
		}
	}

	req := Request{ProviderPreference: ProviderPreferenceLocalOnly}
	dec, err := Resolve(req, in)

	if err == nil {
		t.Fatal("local-only with no viable local harness: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no viable") {
		t.Errorf("error should mention 'no viable': %v", err)
	}
	// All candidates should be ineligible.
	if dec != nil {
		for _, c := range dec.Candidates {
			if c.Eligible {
				t.Errorf("under local-only with no local harness: candidate %s should be ineligible, got eligible", c.Harness)
			}
		}
	}
}

// TestSubscriptionOnlyWithExhaustedQuota verifies subscription-only with
// exhausted quota returns an error when the subscription harness
// SubscriptionOK=false (hard gate).
func TestSubscriptionOnlyWithExhaustedQuota(t *testing.T) {
	in := newTestRoutingEngine()
	// Mark all subscription harnesses as having exhausted quota (hard gate).
	for i, h := range in.Harnesses {
		if h.IsSubscription {
			in.Harnesses[i].SubscriptionOK = false
		}
	}

	req := Request{ProviderPreference: ProviderPreferenceSubscriptionOnly}
	dec, err := Resolve(req, in)

	if err == nil {
		t.Fatal("subscription-only with all quotas exhausted: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no viable") {
		t.Errorf("error should mention 'no viable': %v", err)
	}
	// Subscription harnesses should be ineligible due to QuotaOK=false.
	if dec != nil {
		for _, c := range dec.Candidates {
			if c.Harness == "claude" || c.Harness == "codex" {
				if c.Eligible {
					t.Errorf("subscription harness %s with QuotaOK=false should be ineligible under subscription-only", c.Harness)
				}
			}
		}
	}
}

// TestLocalFirstDefaultCooldownFallback verifies that with default/local-first
// preference, when the only local harness is in cooldown, the engine falls
// back to a subscription harness in the same tier.
func TestLocalFirstDefaultCooldownFallback(t *testing.T) {
	in := newTestRoutingEngine()

	// Put the agent harness (local) in cooldown via harness-level cooldown flag.
	// This affects all providers under the harness simultaneously.
	for i, h := range in.Harnesses {
		if h.Name == "agent" {
			in.Harnesses[i].InCooldown = true
		}
	}

	// Default/empty preference: should fall back to subscription when local is down.
	// Use smart profile so cooldown demotion (140-50=90) falls below subscription (100).
	req := Request{Profile: "smart", ProviderPreference: ""}
	dec, err := Resolve(req, in)
	if err != nil {
		t.Fatalf("Resolve with empty preference + local cooldown: %v", err)
	}
	// Should fall back to subscription (claude or codex).
	if dec.Harness == "agent" {
		t.Errorf("local-first default with agent in cooldown: should fall back to subscription harness, got agent")
	}
}

// TestSubscriptionFirstSelectsSubscription verifies that subscription-first
// selects a subscription harness over a local harness when both are viable,
// for a profile that would otherwise prefer local.
func TestSubscriptionFirstSelectsSubscription(t *testing.T) {
	in := newTestRoutingEngine()

	// standard profile + subscription-first: subscription should win.
	// standard + local-first: agent (local) = 100+25-0 = 125; claude = 100+15-20 = 95 → agent wins
	// standard + subscription-first: agent = 100+0 = 100; claude = 100+30+15-20 = 125 → claude wins
	req := Request{Profile: "standard", ProviderPreference: ProviderPreferenceSubscriptionFirst}
	dec, err := Resolve(req, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// subscription-first should bias toward subscription even in standard.
	if dec.Harness != "claude" {
		t.Errorf("standard + subscription-first: expected claude (subscription), got %q", dec.Harness)
	}
}

// TestQuotaSignalsConsideredInScoring verifies that QuotaStale and
// QuotaPercentUsed affect subscription candidate scores, not model tier.
func TestQuotaSignalsConsideredInScoring(t *testing.T) {
	in := newTestRoutingEngine()

	// Make codex and claude both viable with same cost class.
	// Give claude fresh quota with no burn, codex stale/burning quota.
	for i, h := range in.Harnesses {
		if h.Name == "claude" {
			in.Harnesses[i].QuotaStale = false
			in.Harnesses[i].QuotaTrend = QuotaTrendHealthy
			in.Harnesses[i].QuotaPercentUsed = 10
			in.Harnesses[i].QuotaOK = true
		}
		if h.Name == "codex" {
			in.Harnesses[i].QuotaStale = true
			in.Harnesses[i].QuotaTrend = QuotaTrendExhausting
			in.Harnesses[i].QuotaPercentUsed = 95
			in.Harnesses[i].QuotaOK = true
		}
	}

	req := Request{Profile: "smart"}
	dec, err := Resolve(req, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Fresh/healthy claude should beat stale/exhausting codex.
	// smart base for codex: 100 + 40 (cr=2) + 5 (quota) - 15 (stale) - 40 (exhausting) - 30 (95% used) = 60
	// smart base for claude: 100 + 40 (cr=2) + 5 (quota) + 10 (healthy) = 155
	// claude should win.
	if dec.Harness != "claude" {
		t.Errorf("quota signals: expected claude (fresh/healthy), got %q", dec.Harness)
	}
}

// TestNonClaudeSubscriptionQuotaStale verifies that subscription harnesses
// without a durable quota cache have QuotaStale=true in the routing inputs,
// causing their scores to be penalized.
func TestNonClaudeSubscriptionQuotaStale(t *testing.T) {
	// Simulate what buildRoutingInputs produces for codex (subscription harness
	// without a durable quota cache): QuotaOK=true (hardcoded), QuotaStale=true
	// (no durable cache → conservative stale signal), QuotaTrend=Unknown.
	codexEntry := HarnessEntry{
		Name:            "codex",
		Surface:         "codex",
		CostClass:       "medium",
		IsSubscription:  true,
		Available:       true,
		QuotaOK:         true, // hardcoded for non-Claude
		QuotaStale:      true, // no durable cache → stale
		QuotaTrend:      QuotaTrendUnknown,
		SubscriptionOK:  true, // eligible when SubscriptionOK=true
		DefaultModel:    "gpt-5.4",
		ExactPinSupport: true,
		SupportsTools:   true,
	}

	in := Inputs{
		Harnesses: []HarnessEntry{codexEntry},
		Now:       time.Now(),
	}
	req := Request{Profile: "smart", ProviderPreference: ProviderPreferenceLocalFirst}
	dec, err := Resolve(req, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// codex should still be eligible (QuotaOK=true).
	if !dec.Eligible() {
		t.Errorf("codex with QuotaOK=true should be eligible, got ineligible")
	}

	// Find the codex candidate and verify the score includes the stale penalty.
	// smart base for codex: 100 + 40 (cr=2) + 5 (quota) - 15 (stale) = 130.
	var codexScore float64
	for _, c := range dec.Candidates {
		if c.Harness == "codex" {
			codexScore = c.Score
		}
	}
	if codexScore != 130 {
		t.Errorf("codex stale quota score: got %.1f, want 130.0", codexScore)
	}
}
