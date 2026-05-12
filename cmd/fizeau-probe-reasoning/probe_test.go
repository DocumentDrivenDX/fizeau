package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	reasoning "github.com/easel/fizeau/internal/reasoning"
)

// --- wireFormatFor ---

func TestWireFormatFor(t *testing.T) {
	tests := []struct {
		providerType string
		want         wireFormat
	}{
		{"openrouter", wireOpenRouter},
		{"OPENROUTER", wireOpenRouter},
		{"ds4", wireOpenAIEffort},
		{"anthropic", wireThinkingMap},
		{"lmstudio", wireQwen},
		{"vllm", wireQwen},
		{"llama-server", wireQwen},
		{"omlx", wireQwen},
		{"ollama", wireQwen},
		{"rapid-mlx", wireQwen},
		{"openai-compat", wireQwen},
		{"", wireQwen},
	}
	for _, tt := range tests {
		got := wireFormatFor(tt.providerType)
		if got != tt.want {
			t.Errorf("wireFormatFor(%q) = %q, want %q", tt.providerType, got, tt.want)
		}
	}
}

// --- addReasoningFields ---

func bodyFor(t *testing.T, format wireFormat, policy reasoning.Policy) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{}
	if err := addReasoningFields(body, format, policy); err != nil {
		t.Fatalf("addReasoningFields: %v", err)
	}
	return body
}

func TestAddOpenRouterFields_Off(t *testing.T) {
	body := bodyFor(t, wireOpenRouter, reasoning.Policy{Kind: reasoning.KindOff})
	r, ok := body["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatal("expected reasoning map")
	}
	if r["effort"] != "none" {
		t.Errorf("effort = %v, want none", r["effort"])
	}
}

func TestAddOpenRouterFields_NamedLow(t *testing.T) {
	body := bodyFor(t, wireOpenRouter, reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningLow})
	r := body["reasoning"].(map[string]interface{})
	if r["effort"] != "low" {
		t.Errorf("effort = %v, want low", r["effort"])
	}
}

func TestAddOpenRouterFields_Tokens(t *testing.T) {
	body := bodyFor(t, wireOpenRouter, reasoning.Policy{Kind: reasoning.KindTokens, Value: reasoning.ReasoningTokens(4096), Tokens: 4096})
	r := body["reasoning"].(map[string]interface{})
	if r["max_tokens"] != 4096 {
		t.Errorf("max_tokens = %v, want 4096", r["max_tokens"])
	}
}

func TestAddQwenFields_Off(t *testing.T) {
	body := bodyFor(t, wireQwen, reasoning.Policy{Kind: reasoning.KindOff})
	kw, ok := body["chat_template_kwargs"].(map[string]interface{})
	if !ok {
		t.Fatalf("chat_template_kwargs missing or wrong type: %T", body["chat_template_kwargs"])
	}
	if kw["enable_thinking"] != false {
		t.Errorf("enable_thinking = %v, want false", kw["enable_thinking"])
	}
	if kw["thinking_budget"] != 0 {
		t.Errorf("thinking_budget = %v, want 0", kw["thinking_budget"])
	}
}

func TestAddQwenFields_NamedMedium(t *testing.T) {
	body := bodyFor(t, wireQwen, reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningMedium})
	kw, ok := body["chat_template_kwargs"].(map[string]interface{})
	if !ok {
		t.Fatalf("chat_template_kwargs missing or wrong type: %T", body["chat_template_kwargs"])
	}
	if kw["enable_thinking"] != true {
		t.Errorf("enable_thinking = %v, want true", kw["enable_thinking"])
	}
	// PortableBudgets[medium] = 8192
	if kw["thinking_budget"] != 8192 {
		t.Errorf("thinking_budget = %v, want 8192", kw["thinking_budget"])
	}
}

func TestAddQwenFields_Tokens16384(t *testing.T) {
	body := bodyFor(t, wireQwen, reasoning.Policy{Kind: reasoning.KindTokens, Tokens: 16384})
	kw, ok := body["chat_template_kwargs"].(map[string]interface{})
	if !ok {
		t.Fatalf("chat_template_kwargs missing or wrong type: %T", body["chat_template_kwargs"])
	}
	if kw["thinking_budget"] != 16384 {
		t.Errorf("thinking_budget = %v, want 16384", kw["thinking_budget"])
	}
}

func TestAddOpenAIEffortFields_Off(t *testing.T) {
	body := bodyFor(t, wireOpenAIEffort, reasoning.Policy{Kind: reasoning.KindOff})
	if body["think"] != false {
		t.Errorf("think = %v, want false", body["think"])
	}
	if _, present := body["reasoning_effort"]; present {
		t.Errorf("reasoning_effort should be absent for off")
	}
}

func TestAddOpenAIEffortFields_NamedHigh(t *testing.T) {
	body := bodyFor(t, wireOpenAIEffort, reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningHigh})
	if body["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v, want high", body["reasoning_effort"])
	}
	if _, present := body["think"]; present {
		t.Errorf("think field should be absent for explicit named tier")
	}
}

func TestAddOpenAIEffortFields_Tokens4096(t *testing.T) {
	body := bodyFor(t, wireOpenAIEffort, reasoning.Policy{Kind: reasoning.KindTokens, Tokens: 4096})
	// PortableBudgets are 2048/8192/32768; 4096 is midpoint of low+medium →
	// rounds up to "medium".
	if body["reasoning_effort"] != "medium" {
		t.Errorf("reasoning_effort = %v, want medium (4096 snaps up)", body["reasoning_effort"])
	}
}

func TestAddThinkingMapFields_Off(t *testing.T) {
	body := bodyFor(t, wireThinkingMap, reasoning.Policy{Kind: reasoning.KindOff})
	if _, present := body["thinking"]; present {
		t.Error("thinking field should be absent for off policy")
	}
}

func TestAddThinkingMapFields_NamedHigh(t *testing.T) {
	body := bodyFor(t, wireThinkingMap, reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningHigh})
	th, ok := body["thinking"].(map[string]interface{})
	if !ok {
		t.Fatal("expected thinking map")
	}
	if th["type"] != "enabled" {
		t.Errorf("type = %v, want enabled", th["type"])
	}
	// PortableBudgets[high] = 32768
	if th["budget_tokens"] != 32768 {
		t.Errorf("budget_tokens = %v, want 32768", th["budget_tokens"])
	}
}

// --- buildWireBody ---

func TestBuildWireBody_OpenRouter(t *testing.T) {
	cfg := probeConfig{
		baseURL: "https://openrouter.ai/api/v1",
		model:   "qwen/qwen3.6-27b",
		format:  wireOpenRouter,
	}
	policy := reasoning.Policy{Kind: reasoning.KindTokens, Value: reasoning.ReasoningTokens(4096), Tokens: 4096}
	raw, err := buildWireBody(cfg, policy)
	if err != nil {
		t.Fatalf("buildWireBody: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["model"] != "qwen/qwen3.6-27b" {
		t.Errorf("model = %v", body["model"])
	}
	r := body["reasoning"].(map[string]interface{})
	// json.Unmarshal converts numbers to float64
	if r["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens = %v, want 4096", r["max_tokens"])
	}
}

// --- extractReasoningTokens ---

func TestExtractReasoningTokens(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int
	}{
		{
			name: "standard_openai_nested",
			json: `{"completion_tokens_details":{"reasoning_tokens":1500},"total_tokens":2000}`,
			want: 1500,
		},
		{
			name: "top_level_fallback",
			json: `{"reasoning_tokens":800,"total_tokens":900}`,
			want: 800,
		},
		{
			name: "absent",
			json: `{"total_tokens":100}`,
			want: 0,
		},
		{
			name: "empty",
			json: ``,
			want: 0,
		},
		{
			name: "zero_nested_zero_top",
			json: `{"completion_tokens_details":{"reasoning_tokens":0},"reasoning_tokens":0}`,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReasoningTokens(tt.json)
			if got != tt.want {
				t.Errorf("extractReasoningTokens(%q) = %d, want %d", tt.json, got, tt.want)
			}
		})
	}
}

// --- parseResponse ---

func makeResponseJSON(finishReason string, content string, reasoningToks int) []byte {
	type choice struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	type usage struct {
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	}
	type resp struct {
		Choices []choice        `json:"choices"`
		Usage   json.RawMessage `json:"usage"`
	}

	u := usage{}
	u.CompletionTokensDetails.ReasoningTokens = reasoningToks
	uBytes, _ := json.Marshal(u)

	c := choice{FinishReason: finishReason}
	c.Message.Content = content

	r := resp{
		Choices: []choice{c},
		Usage:   json.RawMessage(uBytes),
	}
	b, _ := json.Marshal(r)
	return b
}

func TestParseResponse_WithThinkBlock(t *testing.T) {
	content := "<think>I should add 2 and 2</think>2+2 equals 4."
	raw := makeResponseJSON("stop", content, 300)

	var r probeResult
	parseResponse(raw, &r)

	if r.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", r.FinishReason)
	}
	if r.ReasoningToks != 300 {
		t.Errorf("ReasoningToks = %d, want 300", r.ReasoningToks)
	}
	if r.ThinkHash == "" {
		t.Error("ThinkHash should be non-empty when <think> block is present")
	}
}

func TestParseResponse_NoThinkBlock(t *testing.T) {
	raw := makeResponseJSON("stop", "2+2 equals 4.", 0)

	var r probeResult
	parseResponse(raw, &r)

	if r.ThinkHash != "" {
		t.Errorf("ThinkHash = %q, want empty (no <think> block)", r.ThinkHash)
	}
}

func TestParseResponse_SameThinkContentProducesSameHash(t *testing.T) {
	content := "<think>same thinking content</think>answer"
	raw := makeResponseJSON("stop", content, 50)

	var r1, r2 probeResult
	parseResponse(raw, &r1)
	parseResponse(raw, &r2)

	if r1.ThinkHash != r2.ThinkHash {
		t.Errorf("same content should produce same hash: %q vs %q", r1.ThinkHash, r2.ThinkHash)
	}
}

// --- computeVerdict ---

func makeResults(toksMap map[string]int) []probeResult {
	results := make([]probeResult, 0, len(probeMatrix))
	for _, pc := range probeMatrix {
		results = append(results, probeResult{
			Label:         pc.Label,
			ReasoningToks: toksMap[pc.Label],
		})
	}
	return results
}

func TestComputeVerdict_AllZero(t *testing.T) {
	results := makeResults(map[string]int{
		"off": 0, "low": 0, "medium": 0, "high": 0, "4096": 0, "16384": 0,
	})
	v := computeVerdict(results)
	if v.Wire != "none" {
		t.Errorf("wire = %q, want none", v.Wire)
	}
	if !v.AllZero {
		t.Error("AllZero should be true")
	}
}

func TestComputeVerdict_FlatNamedTiersTokensWork(t *testing.T) {
	// low/medium/high all produce ~2000 tokens (flat-mapped)
	// 4096 → ~1000, 16384 → ~4000 (proportional)
	results := makeResults(map[string]int{
		"off": 0, "low": 2000, "medium": 2050, "high": 1980, "4096": 1000, "16384": 4000,
	})
	v := computeVerdict(results)
	if v.Wire != "tokens" {
		t.Errorf("wire = %q, want tokens", v.Wire)
	}
	if !v.NamedFlat {
		t.Error("NamedFlat should be true")
	}
	if !v.TokensWork {
		t.Error("TokensWork should be true")
	}
}

func TestComputeVerdict_NamedVaries_TokensWork(t *testing.T) {
	// Named tiers scale meaningfully
	// Token budgets also scale
	results := makeResults(map[string]int{
		"off": 0, "low": 500, "medium": 2000, "high": 8000, "4096": 1000, "16384": 4000,
	})
	v := computeVerdict(results)
	if v.Wire != "effort" {
		t.Errorf("wire = %q, want effort", v.Wire)
	}
	if v.NamedFlat {
		t.Error("NamedFlat should be false when tiers scale")
	}
}

// --- allWithinPct ---

func TestAllWithinPct(t *testing.T) {
	tests := []struct {
		vs   []int
		pct  float64
		want bool
	}{
		{[]int{1000, 1020, 980}, 5, true},   // within ±2% of mean 1000
		{[]int{1000, 2000, 3000}, 5, false}, // far apart
		{[]int{0, 0, 0}, 5, true},           // all zero → trivially flat
		{[]int{}, 5, true},                  // empty → trivially flat
	}
	for _, tt := range tests {
		got := allWithinPct(tt.vs, tt.pct)
		if got != tt.want {
			t.Errorf("allWithinPct(%v, %.0f) = %v, want %v", tt.vs, tt.pct, got, tt.want)
		}
	}
}

// --- budgetsDifferProportionally ---

func TestBudgetsDifferProportionally(t *testing.T) {
	tests := []struct {
		toks1, toks2     int
		budget1, budget2 int
		pct              float64
		want             bool
	}{
		// 4096→1000, 16384→4000: ratio 4.0, expected 4.0 → within 20%
		{1000, 4000, 4096, 16384, 20, true},
		// 4096→1000, 16384→1100: ratio 1.1, expected 4.0 → outside 20%
		{1000, 1100, 4096, 16384, 20, false},
		// zero toks → false
		{0, 4000, 4096, 16384, 20, false},
		{1000, 0, 4096, 16384, 20, false},
	}
	for _, tt := range tests {
		got := budgetsDifferProportionally(tt.toks1, tt.toks2, tt.budget1, tt.budget2, tt.pct)
		if got != tt.want {
			t.Errorf("budgetsDifferProportionally(%d,%d,%d,%d,%.0f) = %v, want %v",
				tt.toks1, tt.toks2, tt.budget1, tt.budget2, tt.pct, got, tt.want)
		}
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestBuildWireBody_SeedAndDeterministic(t *testing.T) {
	cfg := probeConfig{
		baseURL: "https://example.invalid/v1",
		model:   "qwen/qwen3.6-27b",
		format:  wireOpenRouter,
		seed:    4242,
	}
	policy := reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningLow}

	raw1, err := buildWireBody(cfg, policy)
	if err != nil {
		t.Fatalf("buildWireBody first: %v", err)
	}
	raw2, err := buildWireBody(cfg, policy)
	if err != nil {
		t.Fatalf("buildWireBody second: %v", err)
	}
	if string(raw1) != string(raw2) {
		t.Fatalf("buildWireBody should be deterministic for identical inputs:\n%s\n---\n%s", raw1, raw2)
	}

	var body map[string]any
	if err := json.Unmarshal(raw1, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["seed"] != float64(4242) {
		t.Fatalf("seed = %v, want 4242", body["seed"])
	}
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %#v, want one user message", body["messages"])
	}
	msg, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("message = %#v, want object", msgs[0])
	}
	if msg["content"] != probePrompt {
		t.Fatalf("prompt = %v, want %q", msg["content"], probePrompt)
	}
}

func TestUsageListsSnapshotAndMatrixModes(t *testing.T) {
	usage := renderUsage()
	for _, want := range []string{"snapshot-only", "matrix-only", "full (default)", "-seed", "-mode"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage output missing %q:\n%s", want, usage)
		}
	}
}

func TestSnapshotAndMatrixArtifactsFromFixture(t *testing.T) {
	oldClient := probeHTTPClient
	oldNow := probeNow
	probeHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/models/"):
				return testJSONResponse(http.StatusOK, `{
  "id": "qwen/qwen3.6-27b",
  "supported_parameters": ["max_tokens", "reasoning", "seed"],
  "top_provider": {"max_completion_tokens": 81920}
}`), nil
			case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/chat/completions"):
				return testJSONResponse(http.StatusOK, `{
  "choices": [{
    "finish_reason": "stop",
    "message": {"content": "4", "reasoning": "thinking"}
  }],
  "usage": {
    "completion_tokens_details": {"reasoning_tokens": 123}
  }
}`), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			}
		}),
	}
	probeNow = func() time.Time {
		return time.Date(2026, 5, 12, 5, 43, 47, 0, time.UTC)
	}
	defer func() {
		probeHTTPClient = oldClient
		probeNow = oldNow
	}()

	artDir := t.TempDir()
	code := run([]string{
		"--provider", "openrouter",
		"--base-url", "https://example.invalid/api/v1",
		"--model", "qwen/qwen3.6-27b",
		"--artifact-dir", artDir,
		"--mode", "full",
		"--seed", "99",
	})
	if code != 0 {
		t.Fatalf("run returned %d", code)
	}

	for _, name := range []string{"introspection.json", "summary.json", "matrix.md", "off-request.json", "off-response.json"} {
		if _, err := os.Stat(filepath.Join(artDir, name)); err != nil {
			t.Fatalf("expected artifact %s: %v", name, err)
		}
	}

	summaryBytes, err := os.ReadFile(filepath.Join(artDir, "summary.json"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var report probeReport
	if err := json.Unmarshal(summaryBytes, &report); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if len(report.Snapshot) == 0 {
		t.Fatal("summary snapshot should be present")
	}
	if report.Mode != string(probeModeFull) {
		t.Fatalf("mode = %q, want %q", report.Mode, probeModeFull)
	}
	if report.Seed != 99 {
		t.Fatalf("seed = %d, want 99", report.Seed)
	}
	if len(report.Results) != len(probeMatrix) {
		t.Fatalf("results = %d, want %d", len(report.Results), len(probeMatrix))
	}
	if !strings.Contains(string(report.Snapshot), `"supported_parameters"`) {
		t.Fatalf("snapshot missing supported_parameters: %s", report.Snapshot)
	}

	reqBytes, err := os.ReadFile(filepath.Join(artDir, "off-request.json"))
	if err != nil {
		t.Fatalf("read off request: %v", err)
	}
	if !strings.Contains(string(reqBytes), `"seed": 99`) {
		t.Fatalf("off request missing seed:\n%s", reqBytes)
	}
	if !strings.Contains(string(reqBytes), probePrompt) {
		t.Fatalf("off request missing deterministic prompt:\n%s", reqBytes)
	}

	matrixBytes, err := os.ReadFile(filepath.Join(artDir, "matrix.md"))
	if err != nil {
		t.Fatalf("read matrix: %v", err)
	}
	if !strings.Contains(string(matrixBytes), "Verdict") {
		t.Fatalf("matrix output missing verdict:\n%s", matrixBytes)
	}
}
