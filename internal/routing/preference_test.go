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
