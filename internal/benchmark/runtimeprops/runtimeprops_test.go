package runtimeprops_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/easel/fizeau/internal/benchmark/runtimeprops"
)

// --- llamacpp ---

func TestExtractLlamaCPP(t *testing.T) {
	propsResponse := map[string]any{
		"default_generation_settings": map[string]any{
			"n_ctx":          8192,
			"temperature":    0.8,
			"top_p":          0.95,
			"top_k":          40,
			"repeat_penalty": 1.1,
		},
		"model_alias": "Qwen3.6-27B-UD-Q3_K_XL.gguf",
		"build_info":  "b3001 (commit abc1234)",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(propsResponse)
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "llama-server",
		BaseURL: srv.URL + "/v1",
		Model:   "Qwen3.6-27B-UD-Q3_K_XL.gguf",
	}

	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Extractor != "llamacpp" {
		t.Errorf("extractor = %q, want %q", props.Extractor, "llamacpp")
	}
	if props.MaxContext == nil || *props.MaxContext != 8192 {
		t.Errorf("max_context = %v, want 8192", props.MaxContext)
	}
	if props.BuildInfo != "b3001 (commit abc1234)" {
		t.Errorf("build_info = %q", props.BuildInfo)
	}
	if props.SamplingDefaults == nil {
		t.Fatal("sampling_defaults is nil")
	}
	if props.SamplingDefaults.TopK == nil || *props.SamplingDefaults.TopK != 40 {
		t.Errorf("sampling_defaults.top_k = %v, want 40", props.SamplingDefaults.TopK)
	}
	if props.PlatformRaw == nil {
		t.Error("platform_raw is nil")
	}
}

func TestExtractLlamaCPP_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "llamacpp",
		BaseURL: srv.URL + "/v1",
		Model:   "mymodel",
	}

	props, err := runtimeprops.Extract(context.Background(), lane)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if props.ExtractionFailed == "" {
		t.Error("extraction_failed should be set on error")
	}
	if props.ExtractedAt == nil {
		t.Error("extracted_at should be set even on failure")
	}
}

// --- vllm ---

func TestExtractVLLM(t *testing.T) {
	modelsResp := map[string]any{
		"object": "list",
		"data": []any{
			map[string]any{"id": "qwen3.6-27b-autoround", "object": "model"},
		},
	}
	serverInfoResp := map[string]any{
		"version":       "0.4.2",
		"max_model_len": 32768,
		"quantization":  "autoround",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(modelsResp)
		case "/v1/server_info":
			_ = json.NewEncoder(w).Encode(serverInfoResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "vllm",
		BaseURL: srv.URL + "/v1",
		Model:   "qwen3.6-27b-autoround",
	}

	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Extractor != "vllm" {
		t.Errorf("extractor = %q, want vllm", props.Extractor)
	}
	if props.BaseModel != "qwen3.6-27b-autoround" {
		t.Errorf("base_model = %q, want qwen3.6-27b-autoround", props.BaseModel)
	}
	if props.ServerVersion != "0.4.2" {
		t.Errorf("server_version = %q, want 0.4.2", props.ServerVersion)
	}
	if props.MaxContext == nil || *props.MaxContext != 32768 {
		t.Errorf("max_context = %v, want 32768", props.MaxContext)
	}
	if props.ModelQuant != "autoround" {
		t.Errorf("model_quant = %q, want autoround", props.ModelQuant)
	}
}

func TestExtractVLLM_NoServerInfo(t *testing.T) {
	// vLLM without /v1/server_info — should still succeed from /v1/models alone.
	modelsResp := map[string]any{
		"data": []any{
			map[string]any{"id": "mymodel"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(modelsResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{Runtime: "vllm", BaseURL: srv.URL + "/v1", Model: "mymodel"}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.BaseModel != "mymodel" {
		t.Errorf("base_model = %q, want mymodel", props.BaseModel)
	}
}

// --- ds4 ---

func TestExtractDS4(t *testing.T) {
	propsResp := map[string]any{
		"model": map[string]any{
			"id":               "deepseek-v4-flash",
			"mtp":              true,
			"mtp_draft_tokens": 2,
		},
		"runtime": map[string]any{
			"ctx_size": 393216,
		},
		"sampling": map[string]any{
			"defaults": map[string]any{
				"temperature": 1.0,
				"top_p":       1.0,
				"top_k":       0,
			},
		},
	}
	modelsResp := map[string]any{
		"data": []any{
			map[string]any{"id": "deepseek-v4-flash"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/props":
			_ = json.NewEncoder(w).Encode(propsResp)
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(modelsResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "ds4",
		BaseURL: srv.URL + "/v1",
		Model:   "deepseek-v4-flash",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Extractor != "ds4" {
		t.Errorf("extractor = %q, want ds4", props.Extractor)
	}
	if props.BaseModel != "deepseek-v4-flash" {
		t.Errorf("base_model = %q, want deepseek-v4-flash", props.BaseModel)
	}
	if props.MaxContext == nil || *props.MaxContext != 393216 {
		t.Errorf("max_context = %v, want 393216", props.MaxContext)
	}
	if props.MTPEnabled == nil || !*props.MTPEnabled {
		t.Error("mtp_enabled should be true")
	}
	if props.DraftMode != "mtp" {
		t.Errorf("draft_mode = %q, want mtp", props.DraftMode)
	}
	if props.SpeculativeN == nil || *props.SpeculativeN != 2 {
		t.Errorf("speculative_n = %v, want 2", props.SpeculativeN)
	}
	if props.SamplingDefaults == nil {
		t.Fatal("sampling_defaults is nil, want ds4 defaults from /props")
	}
	if props.SamplingDefaults.Temperature == nil || *props.SamplingDefaults.Temperature != 1.0 {
		t.Errorf("temperature = %v, want 1.0", props.SamplingDefaults.Temperature)
	}
	if props.SamplingDefaults.TopP == nil || *props.SamplingDefaults.TopP != 1.0 {
		t.Errorf("top_p = %v, want 1.0", props.SamplingDefaults.TopP)
	}
	if props.SamplingDefaults.TopK == nil || *props.SamplingDefaults.TopK != 0 {
		t.Errorf("top_k = %v, want 0", props.SamplingDefaults.TopK)
	}
}

// --- omlx ---

func TestExtractOMLX(t *testing.T) {
	modelsResp := map[string]any{
		"data": []any{
			map[string]any{
				"id":                 "Qwen3.6-27B-MLX-8bit",
				"max_context_length": 131072,
				"quantization":       "8bit",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(modelsResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "omlx",
		BaseURL: srv.URL + "/v1",
		Model:   "Qwen3.6-27B-MLX-8bit",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Extractor != "omlx" {
		t.Errorf("extractor = %q, want omlx", props.Extractor)
	}
	if props.BaseModel != "Qwen3.6-27B-MLX-8bit" {
		t.Errorf("base_model = %q", props.BaseModel)
	}
	if props.MaxContext == nil || *props.MaxContext != 131072 {
		t.Errorf("max_context = %v, want 131072", props.MaxContext)
	}
	if props.ModelQuant != "8bit" {
		t.Errorf("model_quant = %q, want 8bit", props.ModelQuant)
	}
}

// --- lucebox ---

func TestExtractLucebox_DFlash(t *testing.T) {
	modelsResp := map[string]any{
		"data": []any{
			map[string]any{"id": "luce-dflash"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(modelsResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "lucebox",
		BaseURL: srv.URL + "/v1",
		Model:   "luce-dflash",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.DraftMode != "dflash" {
		t.Errorf("draft_mode = %q, want dflash", props.DraftMode)
	}
}

// --- rapid-mlx ---

func TestExtractRapidMLX(t *testing.T) {
	modelsResp := map[string]any{
		"data": []any{
			map[string]any{"id": "mlx-community/Qwen3.6-27B-8bit"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(modelsResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	lane := runtimeprops.LaneInfo{
		Runtime: "rapid-mlx",
		BaseURL: srv.URL + "/v1",
		Model:   "mlx-community/Qwen3.6-27B-8bit",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Extractor != "rapid-mlx" {
		t.Errorf("extractor = %q, want rapid-mlx", props.Extractor)
	}
	if props.BaseModel != "mlx-community/Qwen3.6-27B-8bit" {
		t.Errorf("base_model = %q", props.BaseModel)
	}
}

// --- cloud / openrouter ---

func TestExtractCloud(t *testing.T) {
	lane := runtimeprops.LaneInfo{
		Runtime: "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "anthropic/claude-sonnet-4.6",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error for cloud lane: %v", err)
	}
	if props.Extractor != "cloud" {
		t.Errorf("extractor = %q, want cloud", props.Extractor)
	}
	if props.BaseModel != "anthropic/claude-sonnet-4.6" {
		t.Errorf("base_model = %q", props.BaseModel)
	}
}

// --- dispatcher ---

func TestDispatcherUnknownRuntime(t *testing.T) {
	lane := runtimeprops.LaneInfo{
		Runtime: "unknown-platform",
		BaseURL: "http://localhost:9999/v1",
		Model:   "somemodel",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unknown runtime should not error: %v", err)
	}
	if props.Extractor != "cloud" {
		t.Errorf("unknown runtime should produce cloud extractor, got %q", props.Extractor)
	}
}

// --- baseURLWithoutV1 is exercised indirectly via llamacpp /props path ---

func TestExtractLlamaCPP_BaseURLNormalization(t *testing.T) {
	// Server serves /props (no /v1 prefix).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/props" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"build_info": "test",
				"default_generation_settings": map[string]any{
					"n_ctx": 4096,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Pass BaseURL with /v1 suffix — extractor must strip it.
	lane := runtimeprops.LaneInfo{
		Runtime: "llama-server",
		BaseURL: srv.URL + "/v1",
		Model:   "mymodel",
	}
	props, err := runtimeprops.Extract(context.Background(), lane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(props.BuildInfo, "test") {
		t.Errorf("expected build_info from props, got %q", props.BuildInfo)
	}
}
