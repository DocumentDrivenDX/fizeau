package runtimeprops

// ds4.go — extractor for DwarfStar 4 (antirez/ds4).
//
// DS4 is an Apple-Silicon MoE inference server. The canonical fixture server
// is vidar:1236. Tested endpoints at time of writing:
//
//   GET /v1/models  — OpenAI-compatible models list (confirmed returns model id)
//   GET /props      — confirmed on live vidar:1236; exposes model.mtp,
//                     model.mtp_draft_tokens, runtime.ctx_size, and sampling
//   GET /health     — health check (TODO: confirm response shape)
//   GET /v1/system_info — (TODO: unverified; may not exist on ds4)
//
// The extractor tries /props first (richest if supported), then falls back to
// /v1/models. MTP is detected from /props model.mtp; there is no separate
// Fizeau env/request toggle for the ds4 benchmark lane.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

func extractDS4(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	base := strings.TrimRight(lane.BaseURL, "/")
	baseNoV1 := baseURLWithoutV1(base)

	now := time.Now().UTC()
	p := evidence.RuntimeProps{
		Extractor:   "ds4",
		ExtractedAt: &now,
		BaseModel:   lane.Model,
	}

	rawData := map[string]any{}

	// Try /props first (llama.cpp-compatible; may be present on ds4).
	propsURL := baseNoV1 + "/props"
	if raw, err := fetchJSON(ctx, propsURL); err == nil {
		rawData["props"] = raw
		var props struct {
			Server struct {
				Name string `json:"name"`
			} `json:"server"`
			Model struct {
				ID             string `json:"id"`
				MTP            *bool  `json:"mtp,omitempty"`
				MTPDraftTokens *int   `json:"mtp_draft_tokens,omitempty"`
			} `json:"model"`
			Runtime struct {
				CtxSize *int `json:"ctx_size,omitempty"`
			} `json:"runtime"`
			Sampling struct {
				Defaults struct {
					Temperature *float64 `json:"temperature,omitempty"`
					TopP        *float64 `json:"top_p,omitempty"`
					TopK        *int     `json:"top_k,omitempty"`
				} `json:"defaults"`
			} `json:"sampling"`
			DefaultGenerationSettings struct {
				NCtx        *int     `json:"n_ctx,omitempty"`
				Temperature *float64 `json:"temperature,omitempty"`
				TopP        *float64 `json:"top_p,omitempty"`
				TopK        *int     `json:"top_k,omitempty"`
			} `json:"default_generation_settings"`
			BuildInfo  string `json:"build_info"`
			MTPEnabled *bool  `json:"mtp_enabled,omitempty"`
		}
		if b, _ := json.Marshal(raw); b != nil {
			_ = json.Unmarshal(b, &props)
		}
		if props.Model.ID != "" {
			p.BaseModel = props.Model.ID
		}
		if props.BuildInfo != "" {
			p.BuildInfo = props.BuildInfo
		}
		switch {
		case props.Runtime.CtxSize != nil && *props.Runtime.CtxSize > 0:
			p.MaxContext = props.Runtime.CtxSize
		case props.DefaultGenerationSettings.NCtx != nil && *props.DefaultGenerationSettings.NCtx > 0:
			p.MaxContext = props.DefaultGenerationSettings.NCtx
		}
		switch {
		case props.Model.MTP != nil:
			p.MTPEnabled = props.Model.MTP
		case props.MTPEnabled != nil:
			p.MTPEnabled = props.MTPEnabled
		}
		if props.Model.MTPDraftTokens != nil && *props.Model.MTPDraftTokens > 0 {
			p.SpeculativeN = props.Model.MTPDraftTokens
		}
		if p.MTPEnabled != nil && *p.MTPEnabled {
			p.DraftMode = "mtp"
		}

		samplingDefaults := evidence.SamplingDefaults{}
		hasSamplingDefaults := false
		if props.Sampling.Defaults.Temperature != nil {
			samplingDefaults.Temperature = props.Sampling.Defaults.Temperature
			hasSamplingDefaults = true
		} else if props.DefaultGenerationSettings.Temperature != nil {
			samplingDefaults.Temperature = props.DefaultGenerationSettings.Temperature
			hasSamplingDefaults = true
		}
		if props.Sampling.Defaults.TopP != nil {
			samplingDefaults.TopP = props.Sampling.Defaults.TopP
			hasSamplingDefaults = true
		} else if props.DefaultGenerationSettings.TopP != nil {
			samplingDefaults.TopP = props.DefaultGenerationSettings.TopP
			hasSamplingDefaults = true
		}
		if props.Sampling.Defaults.TopK != nil {
			samplingDefaults.TopK = props.Sampling.Defaults.TopK
			hasSamplingDefaults = true
		} else if props.DefaultGenerationSettings.TopK != nil {
			samplingDefaults.TopK = props.DefaultGenerationSettings.TopK
			hasSamplingDefaults = true
		}
		if hasSamplingDefaults {
			p.SamplingDefaults = &samplingDefaults
		}
	}

	// Always try /v1/models for the canonical model id.
	modelsURL := baseNoV1 + "/v1/models"
	if raw, err := fetchJSON(ctx, modelsURL); err == nil {
		rawData["models"] = raw
		var models struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if b, _ := json.Marshal(raw); b != nil {
			_ = json.Unmarshal(b, &models)
		}
		if len(models.Data) > 0 && models.Data[0].ID != "" {
			p.BaseModel = models.Data[0].ID
		}
	}

	if len(rawData) == 0 {
		return evidence.RuntimeProps{
			Extractor:        "ds4",
			ExtractedAt:      &now,
			ExtractionFailed: "no endpoints responded at " + base,
		}, nil
	}

	p.PlatformRaw = rawData
	return p, nil
}
