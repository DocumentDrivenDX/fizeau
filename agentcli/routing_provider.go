package agentcli

// routing_provider.go retains only the route-health and ordering helpers
// consumed by routing_smart.go and the route-status command.
//
// As of ADR-005 step 3, the CLI no longer wraps providers with a
// per-Chat failover routeProvider. Provider failover is owned by the
// service-side smart routing engine; the CLI's job is to surface the
// top-scored candidate and let the engine decide on retry. The legacy
// `routeProvider` (an `agentcore.Provider` implementation), its
// `newRouteProvider`/`buildCandidate`/`recordAttempt`/`markCandidateFailure`
// helpers, and the `routeError`/`shouldFailover`/`withModelOverride`
// utilities have all been removed. The structural test
// `TestCLIRoutingProviderHasNoCoreProviderImpl` asserts they do not
// re-enter this file.

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	agentConfig "github.com/DocumentDrivenDX/fizeau/internal/config"
	"github.com/DocumentDrivenDX/fizeau/internal/safefs"
)

const defaultRouteHealthCooldown = 30 * time.Second

type routeHealthState struct {
	Failures map[string]time.Time `json:"failures,omitempty"`
}

func routeAttemptOrder(route routePlanConfig, counter int, state routeHealthState, cooldown time.Duration) []int {
	switch route.Strategy {
	case "priority-round-robin":
		return priorityRoundRobinOrder(route.Candidates, counter, state, cooldown)
	default:
		return orderedFailoverOrder(route.Candidates, state, cooldown)
	}
}

func priorityRoundRobinOrder(candidates []routePlanCandidate, counter int, state routeHealthState, cooldown time.Duration) []int {
	eligible := healthyCandidateIndexes(candidates, state, cooldown)
	if len(eligible) == 0 {
		eligible = make([]int, len(candidates))
		for i := range candidates {
			eligible[i] = i
		}
	}
	bestPriority := candidates[eligible[0]].Priority
	for _, idx := range eligible[1:] {
		if candidates[idx].Priority > bestPriority {
			bestPriority = candidates[idx].Priority
		}
	}
	var preferred []int
	var remainder []int
	for _, idx := range eligible {
		if candidates[idx].Priority == bestPriority {
			preferred = append(preferred, idx)
		} else {
			remainder = append(remainder, idx)
		}
	}
	if len(preferred) == 0 {
		return remainder
	}
	rotated := append([]int(nil), preferred...)
	if len(rotated) > 1 {
		offset := counter % len(rotated)
		rotated = append(rotated[offset:], rotated[:offset]...)
	}
	return append(rotated, remainder...)
}

func orderedFailoverOrder(candidates []routePlanCandidate, state routeHealthState, cooldown time.Duration) []int {
	eligible := healthyCandidateIndexes(candidates, state, cooldown)
	if len(eligible) > 0 {
		return eligible
	}
	order := make([]int, len(candidates))
	for i := range candidates {
		order[i] = i
	}
	return order
}

func healthyCandidateIndexes(candidates []routePlanCandidate, state routeHealthState, cooldown time.Duration) []int {
	now := time.Now().UTC()
	var eligible []int
	for i, candidate := range candidates {
		failedAt, ok := state.Failures[candidate.Provider]
		if !ok || now.Sub(failedAt) >= cooldown {
			eligible = append(eligible, i)
		}
	}
	return eligible
}

func routeHealthStateFile(workDir, routeKey string) string {
	return agentConfig.ProjectRouteHealthPath(workDir, routeStateKey(routeKey))
}

func loadRouteHealthState(workDir, routeKey string) (routeHealthState, error) {
	path := routeHealthStateFile(workDir, routeKey)
	data, err := safefs.ReadFile(path)
	if err != nil {
		return routeHealthState{Failures: make(map[string]time.Time)}, nil
	}
	var state routeHealthState
	if err := json.Unmarshal(data, &state); err != nil {
		return routeHealthState{Failures: make(map[string]time.Time)}, nil
	}
	if state.Failures == nil {
		state.Failures = make(map[string]time.Time)
	}
	return state, nil
}

func saveRouteHealthState(workDir, routeKey string, state routeHealthState) error {
	path := routeHealthStateFile(workDir, routeKey)
	if err := safefs.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	// WriteFileAtomic prevents readers from seeing a partially-written file
	// if the process is interrupted between write and flush.
	return safefs.WriteFileAtomic(path, data, 0o600)
}

func routeHealthCooldown(cfg *agentConfig.Config) time.Duration {
	if cfg == nil || strings.TrimSpace(cfg.Routing.HealthCooldown) == "" {
		return defaultRouteHealthCooldown
	}
	d, err := time.ParseDuration(cfg.Routing.HealthCooldown)
	if err != nil || d <= 0 {
		return defaultRouteHealthCooldown
	}
	return d
}

func routeStateKey(routeName string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", " ", "_")
	return replacer.Replace(routeName)
}
