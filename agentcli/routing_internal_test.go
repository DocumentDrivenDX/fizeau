package agentcli

import (
	"testing"
	"time"

	agentConfig "github.com/easel/fizeau/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestShouldFailover_* and the testError helper were removed as part of
// ADR-005 step 3. The CLI no longer hosts a Chat-level failover wrapper;
// transient-error retry policy moved to the service-side smart-routing
// engine.

func TestHealthyCandidateIndexes_AllHealthy(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1"},
		{Provider: "local-2"},
		{Provider: "cloud"},
	}
	state := routeHealthState{Failures: map[string]time.Time{}}
	cooldown := 30 * time.Second

	indexes := healthyCandidateIndexes(candidates, state, cooldown)
	assert.Equal(t, []int{0, 1, 2}, indexes)
}

func TestHealthyCandidateIndexes_SomeUnhealthy(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1"},
		{Provider: "local-2"},
		{Provider: "cloud"},
	}
	state := routeHealthState{
		Failures: map[string]time.Time{
			"local-1": time.Now().Add(-15 * time.Second), // within cooldown
			"cloud":   time.Now().Add(-60 * time.Second), // outside cooldown
		},
	}
	cooldown := 30 * time.Second

	indexes := healthyCandidateIndexes(candidates, state, cooldown)
	assert.Equal(t, []int{1, 2}, indexes)
}

func TestHealthyCandidateIndexes_AllUnhealthy(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1"},
		{Provider: "local-2"},
	}
	state := routeHealthState{
		Failures: map[string]time.Time{
			"local-1": time.Now().Add(-10 * time.Second),
			"local-2": time.Now().Add(-20 * time.Second),
		},
	}
	cooldown := 30 * time.Second

	indexes := healthyCandidateIndexes(candidates, state, cooldown)
	assert.Empty(t, indexes)
}

func TestPriorityRoundRobinOrder_BasicRotation(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1", Priority: 100},
		{Provider: "local-2", Priority: 100},
		{Provider: "cloud", Priority: 50},
	}
	state := routeHealthState{}
	cooldown := 0 * time.Second

	// First call - local-1 first
	order := priorityRoundRobinOrder(candidates, 0, state, cooldown)
	assert.Equal(t, 0, order[0]) // local-1 first

	// Second call - local-2 first (rotation)
	order = priorityRoundRobinOrder(candidates, 1, state, cooldown)
	assert.Equal(t, 1, order[0]) // local-2 first

	// Third call - local-1 first again
	order = priorityRoundRobinOrder(candidates, 2, state, cooldown)
	assert.Equal(t, 0, order[0]) // local-1 first

	// High priority candidates come before low priority
	assert.Equal(t, 0, order[0]) // local-1 (100)
	assert.Equal(t, 1, order[1]) // local-2 (100)
	assert.Equal(t, 2, order[2]) // cloud (50)
}

func TestPriorityRoundRobinOrder_FiltersUnhealthy(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1", Priority: 100},
		{Provider: "local-2", Priority: 100},
		{Provider: "cloud", Priority: 50},
	}
	state := routeHealthState{
		Failures: map[string]time.Time{
			"local-1": time.Now().Add(-10 * time.Second), // within cooldown
		},
	}
	cooldown := 30 * time.Second

	order := priorityRoundRobinOrder(candidates, 0, state, cooldown)
	// local-1 filtered out, only local-2 and cloud remain
	assert.Equal(t, []int{1, 2}, order)
}

func TestPriorityRoundRobinOrder_AllUnhealthyFallsBack(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1", Priority: 100},
		{Provider: "local-2", Priority: 100},
	}
	state := routeHealthState{
		Failures: map[string]time.Time{
			"local-1": time.Now().Add(-10 * time.Second),
			"local-2": time.Now().Add(-10 * time.Second),
		},
	}
	cooldown := 30 * time.Second

	order := priorityRoundRobinOrder(candidates, 0, state, cooldown)
	// All unhealthy - should return all in original order
	assert.Equal(t, []int{0, 1}, order)
}

func TestOrderedFailoverOrder_AllHealthy(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1"},
		{Provider: "local-2"},
		{Provider: "cloud"},
	}
	state := routeHealthState{}
	cooldown := 30 * time.Second

	order := orderedFailoverOrder(candidates, state, cooldown)
	assert.Equal(t, []int{0, 1, 2}, order)
}

func TestOrderedFailoverOrder_SkipsUnhealthy(t *testing.T) {
	candidates := []routePlanCandidate{
		{Provider: "local-1"},
		{Provider: "local-2"},
		{Provider: "cloud"},
	}
	state := routeHealthState{
		Failures: map[string]time.Time{
			"local-1": time.Now().Add(-10 * time.Second), // within cooldown
		},
	}
	cooldown := 30 * time.Second

	order := orderedFailoverOrder(candidates, state, cooldown)
	assert.Equal(t, []int{1, 2}, order)
}

func TestRouteStateKey(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"qwen3.5-27b", "qwen3.5-27b"},
		{"anthropic/claude-opus-4", "anthropic_claude-opus-4"},
		{"openrouter:code-high", "openrouter_code-high"},
		{"model with spaces", "model_with_spaces"},
	}

	for _, tc := range testCases {
		result := routeStateKey(tc.input)
		assert.Equal(t, tc.expected, result)
	}
}

func TestRouteHealthCooldown_Config(t *testing.T) {
	cfg := &agentConfig.Config{
		Routing: agentConfig.RoutingConfig{
			HealthCooldown: "60s",
		},
	}

	cooldown := routeHealthCooldown(cfg)
	assert.Equal(t, 60*time.Second, cooldown)
}

func TestRouteHealthCooldown_Default(t *testing.T) {
	cfg := &agentConfig.Config{}
	cooldown := routeHealthCooldown(cfg)
	assert.Equal(t, defaultRouteHealthCooldown, cooldown)
}

func TestRouteHealthCooldown_Invalid(t *testing.T) {
	cfg := &agentConfig.Config{
		Routing: agentConfig.RoutingConfig{
			HealthCooldown: "invalid",
		},
	}

	cooldown := routeHealthCooldown(cfg)
	assert.Equal(t, defaultRouteHealthCooldown, cooldown)
}

// TestMax removed: the local `max` helper was deleted with the
// routeProvider in ADR-005 step 3 (Go 1.21 has a builtin).
