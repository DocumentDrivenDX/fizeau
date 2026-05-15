package runtimeprops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

// vllmModelsResponse is the shape of the OpenAI-compatible GET /v1/models response.
type vllmModelsResponse struct {
	Data []struct {
		ID   string `json:"id"`
		Root string `json:"root,omitempty"`
	} `json:"data"`
}

// vllmServerInfo is the shape of GET /v1/server_info (some vLLM forks).
type vllmServerInfo struct {
	Version      string `json:"version,omitempty"`
	ModelName    string `json:"model_name,omitempty"`
	MaxModelLen  int    `json:"max_model_len,omitempty"`
	Quantization string `json:"quantization,omitempty"`
}

func extractVLLM(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	base := strings.TrimRight(lane.BaseURL, "/")
	// Ensure base ends without /v1 for non-v1 paths.
	baseNoV1 := baseURLWithoutV1(base)

	now := time.Now().UTC()
	p := evidence.RuntimeProps{
		Extractor:   "vllm",
		ExtractedAt: &now,
		BaseModel:   lane.Model,
	}

	rawData := map[string]any{}

	// --- GET /v1/models ---
	modelsURL := baseNoV1 + "/v1/models"
	if raw, err := fetchJSON(ctx, modelsURL); err == nil {
		rawData["models"] = raw
		var models vllmModelsResponse
		if b, _ := json.Marshal(raw); b != nil {
			_ = json.Unmarshal(b, &models)
		}
		if len(models.Data) > 0 {
			p.BaseModel = models.Data[0].ID
		}
	}

	// --- GET /v1/server_info (optional, some forks) ---
	serverInfoURL := baseNoV1 + "/v1/server_info"
	if raw, err := fetchJSON(ctx, serverInfoURL); err == nil {
		rawData["server_info"] = raw
		var si vllmServerInfo
		if b, _ := json.Marshal(raw); b != nil {
			_ = json.Unmarshal(b, &si)
		}
		if si.Version != "" {
			p.ServerVersion = si.Version
		}
		if si.MaxModelLen > 0 {
			p.MaxContext = ptrInt(si.MaxModelLen)
		}
		if si.Quantization != "" {
			p.ModelQuant = si.Quantization
		}
	}

	if len(rawData) == 0 {
		return evidence.RuntimeProps{}, fmt.Errorf("vllm: no endpoints responded at %s", base)
	}

	p.PlatformRaw = rawData
	return p, nil
}

// fetchJSON performs a GET request and decodes the response as a
// map[string]any. Returns an error if the request fails or status != 200.
func fetchJSON(ctx context.Context, url string) (map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
