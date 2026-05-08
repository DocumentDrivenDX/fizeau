package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBillingForProviderSystem_AllKnownTypes(t *testing.T) {
	tests := []struct {
		system string
		want   BillingModel
	}{
		{system: "lmstudio", want: BillingModelFixed},
		{system: "llama-server", want: BillingModelFixed},
		{system: "omlx", want: BillingModelFixed},
		{system: "vllm", want: BillingModelFixed},
		{system: "rapid-mlx", want: BillingModelFixed},
		{system: "ollama", want: BillingModelFixed},
		{system: "lucebox", want: BillingModelFixed},
		{system: "openai", want: BillingModelPerToken},
		{system: "openrouter", want: BillingModelPerToken},
		{system: "anthropic", want: BillingModelPerToken},
		{system: "google", want: BillingModelPerToken},
	}

	for _, tt := range tests {
		t.Run(tt.system, func(t *testing.T) {
			assert.Equal(t, tt.want, BillingForProviderSystem(tt.system))
		})
	}
	assert.Equal(t, BillingModelUnknown, BillingForProviderSystem("unknown-provider"))
}

func TestBillingForHarness_SubscriptionHarnesses(t *testing.T) {
	for _, harness := range []string{"claude", "codex", "gemini"} {
		t.Run(harness, func(t *testing.T) {
			assert.Equal(t, BillingModelSubscription, BillingForHarness(harness))
		})
	}
	assert.Equal(t, BillingModelUnknown, BillingForHarness("fiz"))
}
