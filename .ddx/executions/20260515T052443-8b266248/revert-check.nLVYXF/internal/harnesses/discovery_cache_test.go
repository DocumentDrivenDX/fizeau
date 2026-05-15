package harnesses

import (
	"testing"
	"time"

	"github.com/easel/fizeau/internal/modelcatalog"
)

func TestDiscoveryCacheSharedAcrossRunnerDefaultResolutions(t *testing.T) {
	loads := 0
	cache := NewModelDiscoveryCache(func(harnessName, source string) (ModelDiscoverySnapshot, error) {
		loads++
		if harnessName != "codex" {
			t.Fatalf("harnessName = %q, want codex", harnessName)
		}
		if source != EmbeddedDiscoverySource {
			t.Fatalf("source = %q, want %q", source, EmbeddedDiscoverySource)
		}
		return ModelDiscoverySnapshot{
			CapturedAt: time.Now().UTC(),
			Models:     []string{"gpt-5.5", "gpt-5.4"},
			Source:     source,
		}, nil
	})

	first := resolveRunnerModel(cache, "codex", modelcatalog.SurfaceCodex, "", "gpt-5.4")
	second := resolveRunnerModel(cache, "codex", modelcatalog.SurfaceCodex, "", "gpt-5.4")

	if first.ResolvedModel != "gpt-5.5" || second.ResolvedModel != "gpt-5.5" {
		t.Fatalf("resolved models: got %q and %q, want gpt-5.5", first.ResolvedModel, second.ResolvedModel)
	}
	if loads != 1 {
		t.Fatalf("snapshot loads = %d, want 1", loads)
	}
}

func TestResolveRunnerModelExplicitPinWins(t *testing.T) {
	cache := NewModelDiscoveryCache(func(harnessName, source string) (ModelDiscoverySnapshot, error) {
		return ModelDiscoverySnapshot{
			CapturedAt: time.Now().UTC(),
			Models:     []string{"gpt-5.5", "gpt-5.4"},
			Source:     source,
		}, nil
	})

	got := resolveRunnerModel(cache, "codex", modelcatalog.SurfaceCodex, "gpt-5.4", "gpt-5.4")
	if !got.ExplicitPin || got.ResolvedModel != "gpt-5.4" {
		t.Fatalf("resolution = %#v, want explicit gpt-5.4", got)
	}
}
