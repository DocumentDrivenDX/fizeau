package modelcatalog

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := []struct {
		id     string
		family string
		ver    []int
		tier   Tier
		pre    bool
	}{
		{"gpt-5.5", "gpt", []int{5, 5}, TierSmart, false},
		{"gpt-5.5-mini", "gpt", []int{5, 5}, TierStandard, false},
		{"gpt-5.5-nano", "gpt", []int{5, 5}, TierCheap, false},
		{"gpt-5.4", "gpt", []int{5, 4}, TierSmart, false},
		{"gpt-5.4-mini", "gpt", []int{5, 4}, TierStandard, false},
		{"gpt-5", "gpt", []int{5, 0}, TierSmart, false},
		{"gpt-5.5-preview", "gpt", []int{5, 5}, TierSmart, true},
		{"gpt-5.3-codex-spark", "gpt", []int{5, 3}, TierUnknown, false},
		{"claude-opus-4-7", "claude", []int{4, 7}, TierSmart, false},
		{"claude-sonnet-4-6", "claude", []int{4, 6}, TierStandard, false},
		{"claude-haiku-4-5", "claude", []int{4, 5}, TierCheap, false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			p := Parse(tt.id)
			assert.Equal(t, tt.family, p.Family, "family")
			assert.Equal(t, tt.ver, p.Version, "version")
			assert.Equal(t, tt.tier, p.Tier, "tier")
			assert.Equal(t, tt.pre, p.PreRelease, "pre-release")
			assert.Equal(t, tt.id, p.Raw, "raw")
		})
	}
}

func TestParseSort(t *testing.T) {
	sortByRank := func(ids []string) []string {
		models := make([]ParsedModel, len(ids))
		for i, id := range ids {
			models[i] = Parse(id)
		}
		sort.Slice(models, func(i, j int) bool {
			return models[i].Compare(models[j]) < 0
		})
		out := make([]string, len(models))
		for i, m := range models {
			out[i] = m.Raw
		}
		return out
	}

	t.Run("version_and_tier", func(t *testing.T) {
		got := sortByRank([]string{"gpt-5.5-mini", "gpt-5.5", "gpt-5.5-nano", "gpt-5.4"})
		assert.Equal(t, []string{"gpt-5.5", "gpt-5.5-mini", "gpt-5.5-nano", "gpt-5.4"}, got)
	})

	t.Run("prerelease", func(t *testing.T) {
		got := sortByRank([]string{"gpt-5.5-preview", "gpt-5.5"})
		assert.Equal(t, []string{"gpt-5.5", "gpt-5.5-preview"}, got)
	})

	t.Run("claude_short_form", func(t *testing.T) {
		got := sortByRank([]string{"opus-4.6", "opus-4.7", "sonnet-4.7", "haiku-4.7"})
		assert.Equal(t, []string{"opus-4.7", "sonnet-4.7", "haiku-4.7", "opus-4.6"}, got)
	})
}

func TestParseCrossFamily(t *testing.T) {
	a := Parse("gpt-5.5")
	b := Parse("claude-opus-4-7")
	assert.Equal(t, 0, a.Compare(b), "cross-family compare must return 0")
}

func TestParseNoPanic(t *testing.T) {
	inputs := []string{
		"", "unknown", "gpt", "claude", "gemini",
		"gpt-", "-mini", "foo-bar-baz",
		"gpt-abc", "claude-xyz", "gemini-notanumber",
		"opus", "sonnet", "haiku",
	}
	for _, id := range inputs {
		id := id
		assert.NotPanics(t, func() {
			_ = Parse(id)
		}, "Parse must not panic on %q", id)
	}
}

func TestParseUnknownFamilyTierUnknown(t *testing.T) {
	p := Parse("llama-3.3-70b")
	assert.Equal(t, TierUnknown, p.Tier)
	assert.Equal(t, "llama", p.Family)
}
