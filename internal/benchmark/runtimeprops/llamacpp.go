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

// llamaCPPProps is the shape of the llama.cpp GET /props response.
// https://github.com/ggerganov/llama.cpp/blob/master/examples/server/README.md
type llamaCPPProps struct {
	DefaultGenerationSettings struct {
		NCtx          int     `json:"n_ctx"`
		NPredict      int     `json:"n_predict"`
		Temperature   float64 `json:"temperature"`
		TopP          float64 `json:"top_p"`
		TopK          int     `json:"top_k"`
		RepeatPenalty float64 `json:"repeat_penalty"`
	} `json:"default_generation_settings"`
	ModelAlias string `json:"model_alias"`
	ModelPath  string `json:"model_path"`
	TotalSlots int    `json:"total_slots"`
	BuildInfo  string `json:"build_info"`
	// Some builds expose these at top level.
	NCtxTrain int    `json:"n_ctx_train,omitempty"`
	GGMLType  string `json:"ggml_type,omitempty"`
}

func extractLlamaCPP(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	base := strings.TrimRight(lane.BaseURL, "/")
	// /props is served at the root, not under /v1.
	propsURL := baseURLWithoutV1(base) + "/props"

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, propsURL, nil)
	if err != nil {
		return evidence.RuntimeProps{}, fmt.Errorf("llamacpp: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return evidence.RuntimeProps{}, wrapFetchError("llamacpp", propsURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return evidence.RuntimeProps{}, fmt.Errorf("llamacpp: GET %s: status %d", propsURL, resp.StatusCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return evidence.RuntimeProps{}, fmt.Errorf("llamacpp: decode props: %w", err)
	}

	var props llamaCPPProps
	if b, err2 := json.Marshal(raw); err2 == nil {
		_ = json.Unmarshal(b, &props) // best-effort
	}

	now := time.Now().UTC()
	p := evidence.RuntimeProps{
		Extractor:   "llamacpp",
		ExtractedAt: &now,
		BaseModel:   lane.Model,
		BuildInfo:   props.BuildInfo,
		PlatformRaw: raw,
	}

	if props.DefaultGenerationSettings.NCtx > 0 {
		p.MaxContext = ptrInt(props.DefaultGenerationSettings.NCtx)
	}

	sd := &evidence.SamplingDefaults{}
	hasSampling := false
	if props.DefaultGenerationSettings.Temperature != 0 {
		sd.Temperature = ptrFloat64(props.DefaultGenerationSettings.Temperature)
		hasSampling = true
	}
	if props.DefaultGenerationSettings.TopP != 0 {
		sd.TopP = ptrFloat64(props.DefaultGenerationSettings.TopP)
		hasSampling = true
	}
	if props.DefaultGenerationSettings.TopK != 0 {
		sd.TopK = ptrInt(props.DefaultGenerationSettings.TopK)
		hasSampling = true
	}
	if props.DefaultGenerationSettings.RepeatPenalty != 0 {
		sd.RepeatPenalty = ptrFloat64(props.DefaultGenerationSettings.RepeatPenalty)
		hasSampling = true
	}
	if hasSampling {
		p.SamplingDefaults = sd
	}

	return p, nil
}
