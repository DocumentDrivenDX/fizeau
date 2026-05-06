package rapidmlx

import (
	"testing"

	"github.com/DocumentDrivenDX/fizeau/internal/provider/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRapidMLX_DefaultBaseURLAndIdentity(t *testing.T) {
	p := New(Config{Model: "qwen3"})
	require.NotNil(t, p)

	sessionProvider, model := p.SessionStartMetadata()
	assert.Equal(t, "rapid-mlx", sessionProvider)
	assert.Equal(t, "qwen3", model)

	system, host, port := p.ChatStartMetadata()
	assert.Equal(t, "rapid-mlx", system)
	assert.Equal(t, "localhost", host)
	assert.Equal(t, 8000, port)
}

func TestRapidMLX_RegistryRegistration(t *testing.T) {
	d, ok := registry.Lookup("rapid-mlx")
	require.True(t, ok)
	require.NotNil(t, d.Factory)
	assert.Equal(t, DefaultBaseURL, d.DefaultBaseURL)
	assert.Equal(t, 8000, d.DefaultPort)
}
