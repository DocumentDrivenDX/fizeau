package agentcli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/easel/fizeau/internal/modelregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listModelsJSONRow struct {
	Model             string  `json:"model"`
	Harness           string  `json:"harness"`
	Provider          string  `json:"provider"`
	ProviderType      string  `json:"provider_type"`
	EndpointName      string  `json:"endpoint_name"`
	EndpointBaseURL   string  `json:"endpoint_base_url"`
	Endpoint          string  `json:"endpoint"`
	ServerInstance    string  `json:"server_instance"`
	Power             int     `json:"power"`
	AutoRoutable      bool    `json:"auto_routable"`
	ExactPinOnly      bool    `json:"exact_pin_only"`
	CostInputPerMTok  float64 `json:"cost_input_per_mtok"`
	CostOutputPerMTok float64 `json:"cost_output_per_mtok"`
	ContextSource     string  `json:"context_source"`
	SWEBenchVerified  float64 `json:"swe_bench_verified"`
	ContextLength     int     `json:"context_length"`
	PerfSignal        struct {
		SpeedTokensPerSec float64 `json:"speed_tokens_per_sec"`
		SWEBenchVerified  float64 `json:"swe_bench_verified"`
	} `json:"perf_signal"`
	Utilization struct {
		Source         string   `json:"source"`
		Freshness      string   `json:"freshness"`
		ActiveRequests *int     `json:"active_requests"`
		QueuedRequests *int     `json:"queued_requests"`
		MaxConcurrency *int     `json:"max_concurrency"`
		CachePressure  *float64 `json:"cache_pressure"`
	} `json:"utilization"`
	Available bool `json:"available"`
}

func TestCLI_ListModels_TableShowsPowerCostEndpoint(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	local := newCountedOpenAIServer(t, http.StatusOK, "qwen3.5-27b", "ok")
	local.setModels("qwen3.5-27b", "unknown-local")

	writeTempConfig(t, workDir, `
providers:
  studio:
    type: lmstudio
    endpoints:
      - name: vidar
        base_url: `+local.baseURL()+`
default: studio
`)

	res := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--work-dir", workDir, "--list-models")
	require.Equal(t, 0, res.exitCode, "stderr=%s stdout=%s", res.stderr, res.stdout)
	assert.Contains(t, res.stdout, "MODEL")
	assert.Contains(t, res.stdout, "POWER")
	assert.Contains(t, res.stdout, "COST/M")
	assert.Contains(t, res.stdout, "qwen3.5-27b")
	assert.Contains(t, res.stdout, "studio/lmstudio")
	assert.Contains(t, res.stdout, "vidar@")
	assert.Contains(t, res.stdout, "0.10/0.30")
	assert.Contains(t, res.stdout, "SWE 59.0")
	assert.Contains(t, res.stdout, "auto")
	assert.Contains(t, res.stdout, "unknown-local")
}

func TestCLI_ListModels_JSONIncludesRoutingMetadata(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	local := newCountedOpenAIServer(t, http.StatusOK, "qwen3.5-27b", "ok")
	local.setModels("qwen3.5-27b", "unknown-local")

	writeTempConfig(t, workDir, `
providers:
  studio:
    type: lmstudio
    endpoints:
      - name: vidar
        base_url: `+local.baseURL()+`
default: studio
`)

	res := runBuiltCLI(t, exe, workDir, testEnvWithHome(home, nil), "--work-dir", workDir, "--json", "--list-models")
	require.Equal(t, 0, res.exitCode, "stderr=%s stdout=%s", res.stderr, res.stdout)

	var rows []listModelsJSONRow
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &rows), "stdout=%s", res.stdout)
	var generic []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &generic), "stdout=%s", res.stdout)

	known := findListModelsRow(rows, "qwen3.5-27b")
	require.NotNil(t, known, "rows=%s", res.stdout)
	assert.Equal(t, "fiz", known.Harness)
	assert.Equal(t, "studio", known.Provider)
	assert.Equal(t, "lmstudio", known.ProviderType)
	assert.Equal(t, "vidar", known.EndpointName)
	assert.True(t, strings.HasPrefix(known.Endpoint, "vidar@"))
	assert.NotEmpty(t, known.ServerInstance)
	assert.Equal(t, 5, known.Power)
	assert.True(t, known.AutoRoutable)
	assert.False(t, known.ExactPinOnly)
	assert.Equal(t, 0.10, known.CostInputPerMTok)
	assert.Equal(t, 0.30, known.CostOutputPerMTok)
	assert.Equal(t, "catalog", known.ContextSource)
	assert.Equal(t, 59.0, known.SWEBenchVerified)
	assert.Equal(t, 262144, known.ContextLength)
	assert.NotZero(t, known.PerfSignal)
	assert.Empty(t, known.Utilization.Source)
	knownGeneric := findListModelsGenericRow(t, generic, "qwen3.5-27b", res.stdout)
	for _, key := range []string{"server_instance", "context_source", "perf_signal", "utilization"} {
		if _, ok := knownGeneric[key]; !ok {
			t.Fatalf("missing %q in list-models JSON: %s", key, res.stdout)
		}
	}

	unknown := findListModelsRow(rows, "unknown-local")
	require.NotNil(t, unknown, "rows=%s", res.stdout)
	assert.Equal(t, 0, unknown.Power)
	assert.False(t, unknown.AutoRoutable)
	assert.False(t, unknown.ExactPinOnly)
	assert.Equal(t, "vidar", unknown.EndpointName)
}

func TestCLI_ListModels_JSONMatchesModelsJSONFacts(t *testing.T) {
	exe := buildAgentCLI(t)
	workDir := t.TempDir()
	home := t.TempDir()
	cacheDir := t.TempDir()
	local := newCountedOpenAIServer(t, http.StatusOK, "qwen3.5-27b", "ok")
	local.setModels("qwen3.5-27b")

	writeTempConfig(t, workDir, `
providers:
  studio:
    type: lmstudio
    endpoints:
      - name: vidar
        base_url: `+local.baseURL()+`
default: studio
`)

	env := testEnvWithHome(home, map[string]string{
		"PATH":             "",
		"FIZEAU_CACHE_DIR": cacheDir,
	})

	legacy := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "--json", "--list-models", "--provider", "studio")
	require.Equal(t, 0, legacy.exitCode, "stderr=%s stdout=%s", legacy.stderr, legacy.stdout)

	modern := runBuiltCLI(t, exe, workDir, env, "--work-dir", workDir, "models", "--json", "--provider", "studio")
	require.Equal(t, 0, modern.exitCode, "stderr=%s stdout=%s", modern.stderr, modern.stdout)

	var legacyRows []listModelsJSONRow
	require.NoError(t, json.Unmarshal([]byte(legacy.stdout), &legacyRows), "stdout=%s", legacy.stdout)

	var snapshot modelregistry.ModelSnapshot
	require.NoError(t, json.Unmarshal([]byte(modern.stdout), &snapshot), "stdout=%s", modern.stdout)

	legacyFacts := canonicalListModelFacts(legacyRows)
	snapshotFacts := canonicalSnapshotModelFacts(snapshot.Models)
	require.Equal(t, snapshotFacts, legacyFacts)
}

func findListModelsRow(rows []listModelsJSONRow, model string) *listModelsJSONRow {
	for i := range rows {
		if rows[i].Model == model {
			return &rows[i]
		}
	}
	return nil
}

func findListModelsGenericRow(t *testing.T, rows []map[string]json.RawMessage, model, stdout string) map[string]json.RawMessage {
	t.Helper()
	for _, row := range rows {
		var got string
		if err := json.Unmarshal(row["model"], &got); err != nil {
			t.Fatalf("decode model from %s: %v", stdout, err)
		}
		if got == model {
			return row
		}
	}
	t.Fatalf("generic row for %q not found: %s", model, stdout)
	return nil
}

type listModelFacts struct {
	Model             string
	Harness           string
	Provider          string
	ProviderType      string
	EndpointName      string
	EndpointBaseURL   string
	ServerInstance    string
	Power             int
	AutoRoutable      bool
	ExactPinOnly      bool
	CostInputPerMTok  float64
	CostOutputPerMTok float64
	ContextLength     int
}

func canonicalListModelFacts(rows []listModelsJSONRow) map[string]listModelFacts {
	out := make(map[string]listModelFacts, len(rows))
	for _, row := range rows {
		out[listModelFactsKey(row.Provider, row.Model, row.EndpointName, row.EndpointBaseURL, row.ServerInstance)] = listModelFacts{
			Model:             row.Model,
			Harness:           row.Harness,
			Provider:          row.Provider,
			ProviderType:      row.ProviderType,
			EndpointName:      row.EndpointName,
			EndpointBaseURL:   row.EndpointBaseURL,
			ServerInstance:    row.ServerInstance,
			Power:             row.Power,
			AutoRoutable:      row.AutoRoutable,
			ExactPinOnly:      row.ExactPinOnly,
			CostInputPerMTok:  row.CostInputPerMTok,
			CostOutputPerMTok: row.CostOutputPerMTok,
			ContextLength:     row.ContextLength,
		}
	}
	return out
}

func canonicalSnapshotModelFacts(rows []modelregistry.KnownModel) map[string]listModelFacts {
	out := make(map[string]listModelFacts, len(rows))
	for _, row := range rows {
		harness := strings.TrimSpace(row.Harness)
		if harness == "" {
			harness = "fiz"
		}
		out[listModelFactsKey(row.Provider, row.ID, row.EndpointName, row.EndpointBaseURL, row.ServerInstance)] = listModelFacts{
			Model:             row.ID,
			Harness:           harness,
			Provider:          row.Provider,
			ProviderType:      row.ProviderType,
			EndpointName:      row.EndpointName,
			EndpointBaseURL:   row.EndpointBaseURL,
			ServerInstance:    row.ServerInstance,
			Power:             row.Power,
			AutoRoutable:      row.AutoRoutable,
			ExactPinOnly:      row.ExactPinOnly,
			CostInputPerMTok:  row.CostInputPerM,
			CostOutputPerMTok: row.CostOutputPerM,
			ContextLength:     row.ContextWindow,
		}
	}
	return out
}

func listModelFactsKey(provider, model, endpointName, endpointBaseURL, serverInstance string) string {
	return strings.Join([]string{provider, model, endpointName, endpointBaseURL, serverInstance}, "\x00")
}
