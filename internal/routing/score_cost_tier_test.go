package routing

import (
	"math"
	"testing"
)

// TestSonnetBeatsOpusForImplementerPrompt pins fizeau-47a14e1e: under
// policy=default with both per-tier candidates of a multi-tier subscription
// harness in the pool, an implementer-band request (MinPower=8) must select
// the cheaper sonnet tier instead of the higher-power opus tier. The
// scenario mirrors a ~2000-3000-token implementer prompt observed in the
// downstream ddx evidence.
func TestSonnetBeatsOpusForImplementerPrompt(t *testing.T) {
	in := multiTierSubscriptionInputs()
	dec, err := Resolve(Request{Policy: "default", MinPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Harness != "claude" || dec.Model != "sonnet-4.6" {
		t.Fatalf("winner=%s/%s, want claude/sonnet-4.6; candidates=%+v", dec.Harness, dec.Model, dec.Candidates)
	}
	var sonnetScore, opusScore float64
	var sawSonnet, sawOpus bool
	for _, c := range dec.Candidates {
		if c.Harness != "claude" {
			continue
		}
		switch c.Model {
		case "sonnet-4.6":
			sonnetScore = c.Score
			sawSonnet = true
		case "opus-4.7":
			opusScore = c.Score
			sawOpus = true
		}
	}
	if !sawSonnet || !sawOpus {
		t.Fatalf("missing per-tier candidates: sawSonnet=%v sawOpus=%v candidates=%+v", sawSonnet, sawOpus, dec.Candidates)
	}
	if sonnetScore <= opusScore {
		t.Fatalf("sonnet score %.2f <= opus score %.2f; cost-aware default must pick the cheaper in-bounds tier", sonnetScore, opusScore)
	}
}

// TestImplementerScoreComponentsCostDominates pins fizeau-47a14e1e AC2: in
// the same implementer-band scenario, the cost component must be the
// deciding score-component delta between sonnet (Power=8, $0.009/1k) and
// opus (Power=10, $0.045/1k). All other components in scoreComponents
// should tie; cost alone explains sonnet's win.
func TestImplementerScoreComponentsCostDominates(t *testing.T) {
	in := multiTierSubscriptionInputs()
	dec, err := Resolve(Request{Policy: "default", MinPower: 8}, in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var sonnet, opus Candidate
	for _, c := range dec.Candidates {
		if c.Harness != "claude" {
			continue
		}
		switch c.Model {
		case "sonnet-4.6":
			sonnet = c
		case "opus-4.7":
			opus = c
		}
	}
	if sonnet.Model == "" || opus.Model == "" {
		t.Fatalf("missing per-tier candidates: sonnet=%+v opus=%+v", sonnet, opus)
	}
	if sonnet.ScoreComponents == nil || opus.ScoreComponents == nil {
		t.Fatalf("score components must be populated: sonnet=%+v opus=%+v", sonnet.ScoreComponents, opus.ScoreComponents)
	}

	costDelta := sonnet.ScoreComponents["cost"] - opus.ScoreComponents["cost"]
	if costDelta <= 0 {
		t.Fatalf("cost component delta=%.2f (sonnet=%.2f, opus=%.2f); sonnet must score better on cost than opus",
			costDelta, sonnet.ScoreComponents["cost"], opus.ScoreComponents["cost"])
	}

	// Every non-cost component must tie so that cost alone is the deciding
	// factor. If a future change adds another differentiator that helps
	// sonnet, update the assertion deliberately rather than letting silent
	// drift accumulate.
	for _, key := range []string{
		"quota_health", "deployment_locality", "utilization", "performance",
		"power", "context_headroom", "sticky_affinity", "base",
	} {
		if !nearlyEqual(sonnet.ScoreComponents[key], opus.ScoreComponents[key]) {
			t.Fatalf("non-cost component %q differs: sonnet=%.4f opus=%.4f; cost must be the deciding factor",
				key, sonnet.ScoreComponents[key], opus.ScoreComponents[key])
		}
	}

	totalDelta := sonnet.Score - opus.Score
	if !nearlyEqual(totalDelta, costDelta) {
		t.Fatalf("total score delta=%.2f, want it equal to cost-component delta=%.2f", totalDelta, costDelta)
	}
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}
