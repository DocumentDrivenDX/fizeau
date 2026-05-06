package agentcli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listModelsJSONRow struct {
	Model             string  `json:"model"`
	Harness           string  `json:"harness"`
	Provider          string  `json:"provider"`
	ProviderType      string  `json:"provider_type"`
	EndpointName      string  `json:"endpoint_name"`
	Endpoint          string  `json:"endpoint"`
	Power             int     `json:"power"`
	AutoRoutable      bool    `json:"auto_routable"`
	ExactPinOnly      bool    `json:"exact_pin_only"`
	CostInputPerMTok  float64 `json:"cost_input_per_mtok"`
	CostOutputPerMTok float64 `json:"cost_output_per_mtok"`
	SWEBenchVerified  float64 `json:"swe_bench_verified"`
	ContextLength     int     `json:"context_length"`
	Available         bool    `json:"available"`
	CatalogRef        string  `json:"catalog_ref"`
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

	known := findListModelsRow(rows, "qwen3.5-27b")
	require.NotNil(t, known, "rows=%s", res.stdout)
	assert.Equal(t, "fiz", known.Harness)
	assert.Equal(t, "studio", known.Provider)
	assert.Equal(t, "lmstudio", known.ProviderType)
	assert.Equal(t, "vidar", known.EndpointName)
	assert.True(t, strings.HasPrefix(known.Endpoint, "vidar@"))
	assert.Equal(t, 5, known.Power)
	assert.True(t, known.AutoRoutable)
	assert.False(t, known.ExactPinOnly)
	assert.Equal(t, 0.10, known.CostInputPerMTok)
	assert.Equal(t, 0.30, known.CostOutputPerMTok)
	assert.Equal(t, 59.0, known.SWEBenchVerified)
	assert.Equal(t, 262144, known.ContextLength)
	assert.NotEmpty(t, known.CatalogRef)

	unknown := findListModelsRow(rows, "unknown-local")
	require.NotNil(t, unknown, "rows=%s", res.stdout)
	assert.Equal(t, 0, unknown.Power)
	assert.False(t, unknown.AutoRoutable)
	assert.False(t, unknown.ExactPinOnly)
	assert.Empty(t, unknown.CatalogRef)
	assert.Equal(t, "vidar", unknown.EndpointName)
}

func findListModelsRow(rows []listModelsJSONRow, model string) *listModelsJSONRow {
	for i := range rows {
		if rows[i].Model == model {
			return &rows[i]
		}
	}
	return nil
}
