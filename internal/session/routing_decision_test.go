package session

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	agent "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRoutingDecisionFieldsInSessionArtifacts verifies that session start and
// end events record the same routing evidence used by scoring — selected
// endpoint, server instance, sticky state with key/assignment/reason/bonus,
// and utilization state with source/freshness/active/queued counts — so
// callers can inspect routing decisions from session artifacts without parsing
// provider-native responses (AC-3 of FEAT-004).
func TestRoutingDecisionFieldsInSessionArtifacts(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir, "routing-decision-test")

	active := 2
	queued := 1
	bonus := 250.0
	obs := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	start := SessionStartData{
		Provider:               "vllm-provider",
		Model:                  "qwen3-14b",
		SelectedProvider:       "vllm-provider",
		SelectedEndpoint:       "grendel",
		SelectedServerInstance: "grendel",
		Sticky: RoutingStickyState{
			KeyPresent:     true,
			Assignment:     "grendel",
			ServerInstance: "grendel",
			Reason:         "new_assignment",
			Bonus:          bonus,
		},
		Utilization: RoutingUtilizationState{
			Source:         "vllm.metrics",
			Freshness:      "fresh",
			ActiveRequests: &active,
			QueuedRequests: &queued,
			ObservedAt:     obs,
		},
		WorkDir:       "/work",
		MaxIterations: 10,
		Prompt:        "test prompt",
	}
	l.Emit(agent.EventSessionStart, start)

	end := SessionEndData{
		Status:                 agent.StatusSuccess,
		Output:                 "done",
		SelectedProvider:       start.SelectedProvider,
		SelectedEndpoint:       start.SelectedEndpoint,
		SelectedServerInstance: start.SelectedServerInstance,
		Sticky:                 start.Sticky,
		Utilization:            start.Utilization,
	}
	l.Emit(agent.EventSessionEnd, end)

	require.NoError(t, l.Close())

	events, err := ReadEvents(filepath.Join(dir, "routing-decision-test.jsonl"))
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Verify start event routing evidence round-trips correctly.
	var gotStart SessionStartData
	require.NoError(t, json.Unmarshal(events[0].Data, &gotStart))
	assert.Equal(t, "vllm-provider", gotStart.SelectedProvider)
	assert.Equal(t, "grendel", gotStart.SelectedEndpoint)
	assert.Equal(t, "grendel", gotStart.SelectedServerInstance)

	// Sticky state fields.
	assert.True(t, gotStart.Sticky.KeyPresent)
	assert.Equal(t, "grendel", gotStart.Sticky.Assignment)
	assert.Equal(t, "grendel", gotStart.Sticky.ServerInstance)
	assert.Equal(t, "new_assignment", gotStart.Sticky.Reason)
	assert.InDelta(t, bonus, gotStart.Sticky.Bonus, 1e-9)

	// Utilization state fields.
	assert.Equal(t, "vllm.metrics", gotStart.Utilization.Source)
	assert.Equal(t, "fresh", gotStart.Utilization.Freshness)
	require.NotNil(t, gotStart.Utilization.ActiveRequests)
	assert.Equal(t, active, *gotStart.Utilization.ActiveRequests)
	require.NotNil(t, gotStart.Utilization.QueuedRequests)
	assert.Equal(t, queued, *gotStart.Utilization.QueuedRequests)
	assert.True(t, gotStart.Utilization.ObservedAt.Equal(obs))

	// Verify end event carries the same evidence.
	var gotEnd SessionEndData
	require.NoError(t, json.Unmarshal(events[1].Data, &gotEnd))
	assert.Equal(t, "vllm-provider", gotEnd.SelectedProvider)
	assert.Equal(t, "grendel", gotEnd.SelectedEndpoint)
	assert.Equal(t, "grendel", gotEnd.SelectedServerInstance)
	assert.True(t, gotEnd.Sticky.KeyPresent)
	assert.Equal(t, "vllm.metrics", gotEnd.Utilization.Source)
	assert.Equal(t, "fresh", gotEnd.Utilization.Freshness)
	require.NotNil(t, gotEnd.Utilization.ActiveRequests)
	assert.Equal(t, active, *gotEnd.Utilization.ActiveRequests)

	// Verify JSON keys are stable: callers read these without parsing provider responses.
	var startRaw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(events[0].Data, &startRaw))
	for _, key := range []string{
		"selected_provider",
		"selected_endpoint",
		"selected_server_instance",
		"sticky",
		"utilization",
	} {
		assert.Contains(t, startRaw, key, "session.start missing routing evidence key %q", key)
	}

	// Sticky JSON sub-keys.
	var stickyRaw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(startRaw["sticky"], &stickyRaw))
	for _, key := range []string{"key_present", "assignment", "server_instance", "reason", "bonus"} {
		assert.Contains(t, stickyRaw, key, "sticky state missing key %q", key)
	}

	// Utilization JSON sub-keys.
	var utilRaw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(startRaw["utilization"], &utilRaw))
	for _, key := range []string{"source", "freshness", "active_requests", "queued_requests", "observed_at"} {
		assert.Contains(t, utilRaw, key, "utilization state missing key %q", key)
	}

	// Verify end event raw keys also include routing evidence.
	var endRaw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(events[1].Data, &endRaw))
	for _, key := range []string{
		"selected_provider",
		"selected_endpoint",
		"selected_server_instance",
		"sticky",
		"utilization",
	} {
		assert.Contains(t, endRaw, key, "session.end missing routing evidence key %q", key)
	}
}

// TestRoutingDecisionStringFieldsOmittedWhenZero verifies that the string-typed
// routing evidence fields (selected_provider, selected_endpoint,
// selected_server_instance) use omitempty and are absent from JSON when not set.
// Note: Go's encoding/json does not omit zero-value struct fields even with
// omitempty, so Sticky and Utilization always appear; this test covers only the
// string fields.
func TestRoutingDecisionStringFieldsOmittedWhenZero(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir, "routing-zero-test")

	l.Emit(agent.EventSessionStart, SessionStartData{
		Provider:      "anthropic",
		Model:         "claude-sonnet",
		WorkDir:       "/work",
		MaxIterations: 5,
		Prompt:        "hello",
	})
	require.NoError(t, l.Close())

	events, err := ReadEvents(filepath.Join(dir, "routing-zero-test.jsonl"))
	require.NoError(t, err)
	require.Len(t, events, 1)

	var startRaw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(events[0].Data, &startRaw))
	// String fields with omitempty must be absent when zero.
	assert.NotContains(t, startRaw, "selected_provider", "zero selected_provider must be omitted (omitempty)")
	assert.NotContains(t, startRaw, "selected_endpoint", "zero selected_endpoint must be omitted (omitempty)")
	assert.NotContains(t, startRaw, "selected_server_instance", "zero selected_server_instance must be omitted (omitempty)")
}

// TestRoutingDecisionStickyBonusAlwaysPresent verifies that the sticky bonus
// field appears in JSON even when its value is zero — because it uses the
// no-omitempty tag so downstream can distinguish "no bonus" from "not set".
func TestRoutingDecisionStickyBonusAlwaysPresent(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(dir, "sticky-bonus-test")

	// Non-zero sticky so the struct itself isn't omitted, but bonus is zero.
	l.Emit(agent.EventSessionStart, SessionStartData{
		Provider:      "p",
		Model:         "m",
		WorkDir:       "/w",
		MaxIterations: 1,
		Prompt:        "x",
		Sticky: RoutingStickyState{
			Assignment: "vidar", // non-zero keeps struct present
			Bonus:      0,       // zero bonus must still appear
		},
	})
	require.NoError(t, l.Close())

	events, err := ReadEvents(filepath.Join(dir, "sticky-bonus-test.jsonl"))
	require.NoError(t, err)
	require.Len(t, events, 1)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(events[0].Data, &raw))

	require.Contains(t, raw, "sticky", "sticky must be present with non-zero Assignment")
	var sticky map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["sticky"], &sticky))
	assert.Contains(t, sticky, "bonus", "bonus field must be present even when zero (no omitempty)")

	var bonus float64
	require.NoError(t, json.Unmarshal(sticky["bonus"], &bonus))
	assert.Equal(t, 0.0, bonus)
}
