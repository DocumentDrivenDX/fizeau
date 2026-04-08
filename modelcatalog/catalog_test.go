package modelcatalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault_ResolveAlias(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	resolved, err := catalog.Resolve("claude-sonnet", ResolveOptions{
		Surface: SurfaceForgeAnthropic,
	})
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4", resolved.CanonicalID)
	assert.Equal(t, "claude-sonnet", resolved.Ref)
	assert.Equal(t, "claude-sonnet-4-20250514", resolved.ConcreteModel)
	assert.False(t, resolved.Deprecated)
	assert.Equal(t, "embedded", resolved.ManifestSource)
	assert.Equal(t, 1, resolved.ManifestVersion)
}

func TestCurrent_ResolveProfile(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	resolved, err := catalog.Current("code-fast", ResolveOptions{
		Surface: SurfaceForgeOpenAI,
	})
	require.NoError(t, err)
	assert.Equal(t, "code-fast", resolved.Profile)
	assert.Equal(t, "qwen3-coder-next", resolved.CanonicalID)
	assert.Equal(t, "qwen/qwen3-coder-next", resolved.ConcreteModel)
}

func TestResolveCanonicalTarget(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	resolved, err := catalog.Resolve("claude-sonnet-4", ResolveOptions{
		Surface: SurfaceForgeOpenAI,
	})
	require.NoError(t, err)
	assert.Equal(t, "anthropic/claude-sonnet-4", resolved.ConcreteModel)
	assert.Equal(t, "claude-sonnet", resolved.Family)
}

func TestResolveDeprecatedStrict(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	_, err = catalog.Resolve("claude-sonnet-3.7", ResolveOptions{
		Surface: SurfaceForgeAnthropic,
	})
	require.Error(t, err)

	var deprecatedErr *DeprecatedTargetError
	require.True(t, errors.As(err, &deprecatedErr))
	assert.Equal(t, "claude-sonnet-3.7", deprecatedErr.CanonicalID)
	assert.Equal(t, "claude-sonnet-4", deprecatedErr.Replacement)
}

func TestResolveDeprecatedAllowed(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	resolved, err := catalog.Resolve("claude-sonnet-3.7", ResolveOptions{
		Surface:         SurfaceForgeAnthropic,
		AllowDeprecated: true,
	})
	require.NoError(t, err)
	assert.True(t, resolved.Deprecated)
	assert.Equal(t, "claude-sonnet-4", resolved.Replacement)
	assert.Equal(t, "claude-3-7-sonnet-20250219", resolved.ConcreteModel)
}

func TestLoad_ExternalOverride(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "models.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`
version: 2
generated_at: 2026-04-09T00:00:00Z
profiles:
  code-smart:
    target: gpt-4.1
targets:
  gpt-4.1:
    family: gpt-4.1
    aliases: [gpt-smart]
    surfaces:
      forge.openai: gpt-4.1
`), 0o644))

	catalog, err := Load(LoadOptions{ManifestPath: manifestPath})
	require.NoError(t, err)

	resolved, err := catalog.Resolve("gpt-smart", ResolveOptions{
		Surface: SurfaceForgeOpenAI,
	})
	require.NoError(t, err)
	assert.Equal(t, "gpt-4.1", resolved.CanonicalID)
	assert.Equal(t, "gpt-4.1", resolved.ConcreteModel)
	assert.Equal(t, manifestPath, resolved.ManifestSource)
	assert.Equal(t, 2, resolved.ManifestVersion)
}

func TestLoad_FallbackToEmbedded(t *testing.T) {
	catalog, err := Load(LoadOptions{
		ManifestPath: filepath.Join(t.TempDir(), "missing.yaml"),
	})
	require.NoError(t, err)

	resolved, err := catalog.Resolve("code-smart", ResolveOptions{
		Surface: SurfaceForgeAnthropic,
	})
	require.NoError(t, err)
	assert.Equal(t, "embedded", resolved.ManifestSource)
	assert.Equal(t, "claude-sonnet-4", resolved.CanonicalID)
}

func TestLoad_RequireExternal(t *testing.T) {
	_, err := Load(LoadOptions{
		ManifestPath:    filepath.Join(t.TempDir(), "missing.yaml"),
		RequireExternal: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read manifest")
}

func TestLoad_InvalidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "models.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(`
version: 1
generated_at: 2026-04-09T00:00:00Z
profiles:
  code-smart:
    target: missing
targets:
  claude-sonnet-4:
    family: claude-sonnet
    aliases: [dup]
    surfaces:
      forge.anthropic: claude-sonnet-4-20250514
  qwen3-coder-next:
    family: qwen3-coder
    aliases: [dup]
    surfaces:
      forge.openai: qwen/qwen3-coder-next
`), 0o644))

	_, err := Load(LoadOptions{
		ManifestPath:    manifestPath,
		RequireExternal: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collides")
}

func TestResolveMissingSurface(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	_, err = catalog.Resolve("qwen3-coder-next", ResolveOptions{
		Surface: SurfaceForgeAnthropic,
	})
	require.Error(t, err)

	var missingSurfaceErr *MissingSurfaceError
	require.True(t, errors.As(err, &missingSurfaceErr))
	assert.Equal(t, SurfaceForgeAnthropic, missingSurfaceErr.Surface)
}

func TestResolveUnknownReference(t *testing.T) {
	catalog, err := Default()
	require.NoError(t, err)

	_, err = catalog.Resolve("does-not-exist", ResolveOptions{
		Surface: SurfaceForgeOpenAI,
	})
	require.Error(t, err)

	var unknownErr *UnknownReferenceError
	require.True(t, errors.As(err, &unknownErr))
	assert.Equal(t, "does-not-exist", unknownErr.Ref)
}
