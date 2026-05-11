// Command fizeau-probe-reasoning measures which reasoning wire knob a live
// endpoint actually honors. It sends a fixed 6-row matrix of chat-completion
// requests (off / low / medium / high / 4096 / 16384) and prints a markdown
// table with per-row measurements and a verdict line recommending the correct
// reasoning_wire catalog value.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/profile"
	reasoning "github.com/easel/fizeau/internal/reasoning"
)

const (
	probePrompt    = "Briefly explain what 2+2 equals."
	probeMaxToks   = 512 // enough for an answer + some thinking
	requestTimeout = 5 * time.Minute
)

// wireFormat enumerates the provider-side reasoning wire shapes the probe can emit.
type wireFormat string

const (
	wireOpenRouter   wireFormat = "openrouter"    // reasoning: {effort|max_tokens}
	wireQwen         wireFormat = "qwen"          // chat_template_kwargs.{enable_thinking, thinking_budget}
	wireThinkingMap  wireFormat = "thinking_map"  // thinking: {type, budget_tokens}
	wireOpenAIEffort wireFormat = "openai_effort" // top-level reasoning_effort + think:false off path (ds4)
)

// probeCase is one row in the measurement matrix.
type probeCase struct {
	Label  string
	Policy reasoning.Policy
}

// probeMatrix is the fixed set of reasoning requests to send.
var probeMatrix = []probeCase{
	{Label: "off", Policy: reasoning.Policy{Kind: reasoning.KindOff, Value: reasoning.ReasoningOff}},
	{Label: "low", Policy: reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningLow}},
	{Label: "medium", Policy: reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningMedium}},
	{Label: "high", Policy: reasoning.Policy{Kind: reasoning.KindNamed, Value: reasoning.ReasoningHigh}},
	{Label: "4096", Policy: reasoning.Policy{Kind: reasoning.KindTokens, Value: reasoning.ReasoningTokens(4096), Tokens: 4096}},
	{Label: "16384", Policy: reasoning.Policy{Kind: reasoning.KindTokens, Value: reasoning.ReasoningTokens(16384), Tokens: 16384}},
}

// probeResult captures the measured outcome for one matrix row.
type probeResult struct {
	Label                 string
	WireBody              json.RawMessage // raw JSON sent to the endpoint
	FinishReason          string
	ReasoningToks         int
	ReasoningTokensApprox bool // true when derived from len(reasoning_content)/4
	WallTime              time.Duration
	ThinkHash             string // SHA-256 hex prefix of <think> or reasoning_content, or ""
	ResponseBody          json.RawMessage
	Error                 string
}

// probeConfig holds the resolved endpoint / provider parameters.
type probeConfig struct {
	baseURL   string
	apiKey    string
	model     string
	format    wireFormat
	profileID string
}

// verdict is the analysis result over all matrix rows.
type verdict struct {
	Wire        string // recommended catalog reasoning_wire value
	Explanation string
	AllZero     bool
	NamedFlat   bool // low/medium/high produce same token count
	TokensWork  bool // 4096 vs 16384 differ proportionally
}

// probeReport bundles results + verdict for serialisation.
type probeReport struct {
	ProfileID  string        `json:"profile_id"`
	Model      string        `json:"model"`
	BaseURL    string        `json:"base_url"`
	WireFormat string        `json:"wire_format"`
	Timestamp  string        `json:"timestamp"`
	Results    []probeResult `json:"results"`
	Verdict    verdict       `json:"verdict"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("fizeau-probe-reasoning", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	profilePath := fs.String("profile", "", "Path to fizeau benchmark profile YAML")
	providerFlag := fs.String("provider", "", "Provider type override (openrouter|lmstudio|vllm|etc.)")
	modelFlag := fs.String("model", "", "Model ID override")
	baseURLFlag := fs.String("base-url", "", "API base URL override")
	apiKeyFlag := fs.String("api-key", "", "API key (overrides api_key_env from profile)")
	jsonOut := fs.Bool("json", false, "Emit machine-readable JSON instead of markdown")
	artifactDir := fs.String("artifact-dir", "", "Directory for per-row audit files (default: /tmp/probe-<ts>/)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := resolveConfig(*profilePath, *providerFlag, *modelFlag, *baseURLFlag, *apiKeyFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fs.Usage()
		return 2
	}

	artDir := *artifactDir
	if artDir == "" {
		artDir = filepath.Join("/tmp", fmt.Sprintf("probe-%s", time.Now().UTC().Format("20060102T150405Z")))
	}
	if err := os.MkdirAll(artDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "error: create artifact dir %s: %v\n", artDir, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "probe: model=%s base_url=%s wire=%s\n", cfg.model, cfg.baseURL, cfg.format)
	fmt.Fprintf(os.Stderr, "probe: artifacts → %s\n", artDir)

	results := runMatrix(cfg, artDir)
	v := computeVerdict(results)

	report := probeReport{
		ProfileID:  cfg.profileID,
		Model:      cfg.model,
		BaseURL:    cfg.baseURL,
		WireFormat: string(cfg.format),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Results:    results,
		Verdict:    v,
	}

	// Write a summary artifact.
	writeArtifact(artDir, "summary.json", mustMarshalIndent(report))

	if *jsonOut {
		return outputJSON(report)
	}
	return outputMarkdown(report)
}

// resolveConfig builds a probeConfig from a profile path or explicit flags.
func resolveConfig(profilePath, providerType, model, baseURL, apiKey string) (probeConfig, error) {
	if profilePath != "" {
		p, err := profile.Load(profilePath)
		if err != nil {
			return probeConfig{}, fmt.Errorf("load profile: %w", err)
		}
		// Explicit flag overrides win over profile values.
		if model == "" {
			model = p.Provider.Model
		}
		if baseURL == "" {
			baseURL = p.Provider.BaseURL
		}
		if providerType == "" {
			providerType = string(p.Provider.Type)
		}
		if apiKey == "" && p.Provider.APIKeyEnv != "" {
			apiKey = os.Getenv(p.Provider.APIKeyEnv)
		}
		cfg := probeConfig{
			baseURL:   baseURL,
			apiKey:    apiKey,
			model:     model,
			format:    wireFormatFor(providerType),
			profileID: p.ID,
		}
		return cfg, cfg.validate()
	}

	// Flags-only mode.
	cfg := probeConfig{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		format:  wireFormatFor(providerType),
	}
	return cfg, cfg.validate()
}

func (c probeConfig) validate() error {
	if c.baseURL == "" {
		return fmt.Errorf("--base-url (or profile.provider.base_url) is required")
	}
	if c.model == "" {
		return fmt.Errorf("--model (or profile.provider.model) is required")
	}
	return nil
}

// wireFormatFor maps a profile provider type string to the correct wire format.
func wireFormatFor(providerType string) wireFormat {
	switch strings.ToLower(providerType) {
	case "openrouter":
		return wireOpenRouter
	case "ds4":
		return wireOpenAIEffort
	case "anthropic":
		return wireThinkingMap
	default:
		// lmstudio, vllm, llama-server, omlx, ollama, rapid-mlx, openai-compat use Qwen wire.
		return wireQwen
	}
}

// runMatrix sends all probeMatrix rows sequentially and returns the results.
func runMatrix(cfg probeConfig, artDir string) []probeResult {
	results := make([]probeResult, 0, len(probeMatrix))
	for _, pc := range probeMatrix {
		fmt.Fprintf(os.Stderr, "probe: sending reasoning=%s ...\n", pc.Label)
		r := sendProbe(cfg, pc)
		writeArtifacts(artDir, pc.Label, r)
		results = append(results, r)
	}
	return results
}

// sendProbe executes one chat-completion request and returns the measured result.
func sendProbe(cfg probeConfig, pc probeCase) probeResult {
	r := probeResult{Label: pc.Label}

	body, err := buildWireBody(cfg, pc.Policy)
	if err != nil {
		r.Error = fmt.Sprintf("build wire body: %v", err)
		return r
	}
	r.WireBody = body

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	endpoint := strings.TrimRight(cfg.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		r.Error = fmt.Sprintf("build request: %v", err)
		return r
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	r.WallTime = time.Since(start)
	if err != nil {
		r.Error = fmt.Sprintf("http: %v", err)
		return r
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		r.Error = fmt.Sprintf("read response: %v", err)
		return r
	}
	r.ResponseBody = json.RawMessage(respBytes)

	if resp.StatusCode != http.StatusOK {
		r.Error = fmt.Sprintf("http %d: %s", resp.StatusCode, truncate(string(respBytes), 200))
		return r
	}

	parseResponse(respBytes, &r)
	return r
}

// buildWireBody constructs the chat-completion request JSON for a given policy
// and wire format.
func buildWireBody(cfg probeConfig, policy reasoning.Policy) (json.RawMessage, error) {
	body := map[string]interface{}{
		"model": cfg.model,
		"messages": []map[string]string{
			{"role": "user", "content": probePrompt},
		},
		"max_tokens": probeMaxToks,
	}

	if err := addReasoningFields(body, cfg.format, policy); err != nil {
		return nil, err
	}

	out, err := json.MarshalIndent(body, "", "  ")
	return json.RawMessage(out), err
}

// addReasoningFields injects the wire-format-specific reasoning fields into the
// request body map. It is a pure function used both by sendProbe and tests.
func addReasoningFields(body map[string]interface{}, format wireFormat, policy reasoning.Policy) error {
	switch format {
	case wireOpenRouter:
		return addOpenRouterFields(body, policy)
	case wireQwen:
		return addQwenFields(body, policy)
	case wireThinkingMap:
		return addThinkingMapFields(body, policy)
	case wireOpenAIEffort:
		return addOpenAIEffortFields(body, policy)
	default:
		return fmt.Errorf("unknown wire format %q", format)
	}
}

func addOpenRouterFields(body map[string]interface{}, policy reasoning.Policy) error {
	switch policy.Kind {
	case reasoning.KindOff:
		body["reasoning"] = map[string]interface{}{"effort": "none"}
	case reasoning.KindNamed:
		effort := string(policy.Value)
		switch effort {
		case "minimal", "low", "medium", "high", "xhigh":
		default:
			return fmt.Errorf("unsupported OpenRouter effort %q", effort)
		}
		body["reasoning"] = map[string]interface{}{"effort": effort}
	case reasoning.KindTokens:
		body["reasoning"] = map[string]interface{}{"max_tokens": policy.Tokens}
	default:
		return fmt.Errorf("unsupported policy kind %q for openrouter wire", policy.Kind)
	}
	return nil
}

func addQwenFields(body map[string]interface{}, policy reasoning.Policy) error {
	// llama-server / vLLM Qwen3 require chat_template_kwargs envelope; top-level
	// enable_thinking/thinking_budget is silently dropped (verified 2026-05-11
	// against sindri llama.cpp). Mirrors fizeau translator at
	// internal/provider/openai/openai.go (cfdcdcc4).
	if policy.Kind == reasoning.KindOff {
		body["chat_template_kwargs"] = map[string]interface{}{
			"enable_thinking": false,
			"thinking_budget": 0,
		}
		return nil
	}
	budget, err := reasoning.BudgetFor(policy, nil, 0)
	if err != nil {
		return err
	}
	if budget <= 0 {
		body["chat_template_kwargs"] = map[string]interface{}{
			"enable_thinking": false,
			"thinking_budget": 0,
		}
		return nil
	}
	body["chat_template_kwargs"] = map[string]interface{}{
		"enable_thinking": true,
		"thinking_budget": budget,
	}
	return nil
}

// addOpenAIEffortFields emits flat top-level reasoning_effort:"<tier>" + think:false
// off path. Used by ds4 (deepseek-v4-flash) per /props.api.supported_request_parameters.
// Mirrors fizeau translator's ThinkingWireFormatOpenAIEffort branch (cfdcdcc4).
func addOpenAIEffortFields(body map[string]interface{}, policy reasoning.Policy) error {
	if policy.Kind == reasoning.KindOff {
		body["think"] = false
		return nil
	}
	switch policy.Kind {
	case reasoning.KindNamed:
		effort := string(policy.Value)
		switch effort {
		case "minimal", "low", "medium", "high", "xhigh", "max":
			body["reasoning_effort"] = effort
		default:
			return fmt.Errorf("unsupported OpenAIEffort tier %q", effort)
		}
	case reasoning.KindTokens:
		// Snap to nearest tier via PortableBudgets, matching fizeau's
		// NearestTierForTokens behavior (round up on ties).
		tier := reasoning.NearestTierForTokens(policy.Tokens)
		body["reasoning_effort"] = string(tier)
	default:
		return fmt.Errorf("unsupported policy kind %q for openai_effort wire", policy.Kind)
	}
	return nil
}

func addThinkingMapFields(body map[string]interface{}, policy reasoning.Policy) error {
	if policy.Kind == reasoning.KindOff {
		// Some servers expect the field absent; others accept type=disabled.
		// Send no field for off so we don't confuse non-thinking models.
		return nil
	}
	budget, err := reasoning.BudgetFor(policy, nil, 0)
	if err != nil {
		return err
	}
	if budget <= 0 {
		return nil
	}
	body["thinking"] = map[string]interface{}{
		"type":          "enabled",
		"budget_tokens": budget,
	}
	return nil
}

var thinkRe = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// parseResponse extracts finish_reason, reasoning_tokens, and <think> hash
// from a raw chat-completion response body.
func parseResponse(raw []byte, r *probeResult) {
	var resp struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		r.Error = fmt.Sprintf("parse response json: %v", err)
		return
	}
	var reasoningContent string
	if len(resp.Choices) > 0 {
		r.FinishReason = resp.Choices[0].FinishReason
		content := resp.Choices[0].Message.Content
		reasoningContent = resp.Choices[0].Message.ReasoningContent
		// Hash inline <think> when reasoning_format=none leaves it in content;
		// otherwise hash the extracted reasoning_content.
		if m := thinkRe.FindStringSubmatch(content); len(m) > 1 {
			sum := sha256.Sum256([]byte(m[1]))
			r.ThinkHash = fmt.Sprintf("%x", sum[:8])
		} else if reasoningContent != "" {
			sum := sha256.Sum256([]byte(reasoningContent))
			r.ThinkHash = fmt.Sprintf("%x", sum[:8])
		}
	}
	if len(resp.Usage) > 0 {
		r.ReasoningToks = extractReasoningTokens(string(resp.Usage))
	}
	// Fallback when usage.completion_tokens_details.reasoning_tokens is absent
	// but the response carries reasoning_content (ds4, some llama-server builds):
	// approximate via chars/4. Mirrors fizeau-8f62bcbb behavior so probe and
	// benchmark cells report comparable values.
	if r.ReasoningToks == 0 && reasoningContent != "" {
		r.ReasoningToks = len(reasoningContent) / 4
		r.ReasoningTokensApprox = true
	}
}

// extractReasoningTokens mirrors the same function in internal/sdk/openaicompat.
func extractReasoningTokens(rawUsageJSON string) int {
	if rawUsageJSON == "" {
		return 0
	}
	var raw struct {
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
	}
	if err := json.Unmarshal([]byte(rawUsageJSON), &raw); err != nil {
		return 0
	}
	if raw.ReasoningTokens != nil && *raw.ReasoningTokens > 0 {
		return *raw.ReasoningTokens
	}
	return raw.CompletionTokensDetails.ReasoningTokens
}

// computeVerdict analyses the matrix results and returns a catalog recommendation.
func computeVerdict(results []probeResult) verdict {
	toks := map[string]int{}
	for _, r := range results {
		toks[r.Label] = r.ReasoningToks
	}

	// Edge: no reasoning tokens at all.
	allZero := true
	for _, r := range results {
		if r.ReasoningToks > 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return verdict{
			Wire:        "none",
			Explanation: "reasoning_tokens is 0 for every matrix row; the upstream endpoint is not performing any reasoning. Flag the model as reasoning_wire=none in the catalog.",
			AllZero:     true,
		}
	}

	// Check whether named tiers (low/medium/high) are flat-mapped.
	namedToks := []int{toks["low"], toks["medium"], toks["high"]}
	namedFlat := allWithinPct(namedToks, 5)

	// Check whether token budgets (4096 vs 16384) produce proportionally different counts.
	tok4096 := toks["4096"]
	tok16384 := toks["16384"]
	tokensWork := budgetsDifferProportionally(tok4096, tok16384, 4096, 16384, 20)

	switch {
	case namedFlat && tokensWork:
		return verdict{
			Wire:        "tokens",
			Explanation: "Named effort tiers (low/medium/high) produce the same reasoning_token count — they are flat-mapped upstream. Token budgets (4096 vs 16384) are honored. Use reasoning_wire=tokens in the catalog.",
			NamedFlat:   true,
			TokensWork:  true,
		}
	case namedFlat && !tokensWork:
		return verdict{
			Wire:        "tokens",
			Explanation: "Named effort tiers are flat-mapped upstream. Token budget variation was not observed (possibly capped or not yet testable). Recommend reasoning_wire=tokens as the safer wire form.",
			NamedFlat:   true,
			TokensWork:  false,
		}
	case !namedFlat && tokensWork:
		return verdict{
			Wire:        "effort",
			Explanation: "Both effort tiers and token budgets vary meaningfully. Either wire form works; defaulting to reasoning_wire=effort (the more natural caller form).",
			NamedFlat:   false,
			TokensWork:  true,
		}
	default:
		return verdict{
			Wire:        "effort",
			Explanation: "Named tiers vary but token budgets do not show clear proportional scaling. Use reasoning_wire=effort; investigate reasoning_wire=tokens if token precision is needed.",
			NamedFlat:   false,
			TokensWork:  false,
		}
	}
}

// allWithinPct returns true if all values in vs are within pct% of their mean.
func allWithinPct(vs []int, pct float64) bool {
	if len(vs) == 0 {
		return true
	}
	nonZero := 0
	sum := 0
	for _, v := range vs {
		if v > 0 {
			nonZero++
			sum += v
		}
	}
	if nonZero == 0 {
		return true
	}
	mean := float64(sum) / float64(nonZero)
	for _, v := range vs {
		if v == 0 {
			continue
		}
		if math.Abs(float64(v)-mean)/mean*100 > pct {
			return false
		}
	}
	return true
}

// budgetsDifferProportionally returns true when the ratio of observed reasoning
// tokens for two different budgets is within tolerancePct% of the ratio of the
// requested budgets. E.g. budget 16384 should produce ~4x the tokens of 4096.
func budgetsDifferProportionally(toks1, toks2, budget1, budget2 int, tolerancePct float64) bool {
	if toks1 <= 0 || toks2 <= 0 {
		return false
	}
	expectedRatio := float64(budget2) / float64(budget1)
	observedRatio := float64(toks2) / float64(toks1)
	// Within tolerancePct% of expected ratio.
	return math.Abs(observedRatio-expectedRatio)/expectedRatio*100 <= tolerancePct
}

func outputMarkdown(report probeReport) int {
	fmt.Printf("# fizeau-probe-reasoning\n\n")
	fmt.Printf("**Profile:** %s  \n", report.ProfileID)
	fmt.Printf("**Model:** `%s`  \n", report.Model)
	fmt.Printf("**Endpoint:** `%s`  \n", report.BaseURL)
	fmt.Printf("**Wire format:** `%s`  \n", report.WireFormat)
	fmt.Printf("**Timestamp:** %s\n\n", report.Timestamp)

	fmt.Printf("| Reasoning | finish_reason | reasoning_tokens | wall_time | think_hash | error |\n")
	fmt.Printf("|-----------|---------------|-----------------|-----------|------------|-------|\n")
	for _, r := range report.Results {
		errCol := r.Error
		if errCol == "" {
			errCol = "—"
		}
		thinkCol := r.ThinkHash
		if thinkCol == "" {
			thinkCol = "—"
		}
		fmt.Printf("| `%s` | %s | %d | %s | `%s` | %s |\n",
			r.Label,
			r.FinishReason,
			r.ReasoningToks,
			r.WallTime.Round(time.Millisecond),
			thinkCol,
			errCol,
		)
	}

	v := report.Verdict
	fmt.Printf("\n**Verdict:** recommended `reasoning_wire=%s`\n\n> %s\n", v.Wire, v.Explanation)
	return 0
}

func outputJSON(report probeReport) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "error: encode json: %v\n", err)
		return 1
	}
	return 0
}

// writeArtifacts saves the request and response bodies for one matrix row.
func writeArtifacts(artDir, label string, r probeResult) {
	writeArtifact(artDir, label+"-request.json", prettyJSON(r.WireBody))
	writeArtifact(artDir, label+"-response.json", prettyJSON(r.ResponseBody))
}

func writeArtifact(artDir, name string, data []byte) {
	path := filepath.Join(artDir, name)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		fmt.Fprintf(os.Stderr, "warn: write artifact %s: %v\n", path, err)
	}
}

func prettyJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null\n")
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return raw
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

func mustMarshalIndent(v interface{}) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return append(b, '\n')
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
