package runtimeprops

// ds4.go — extractor for DwarfStar 4 (antirez/ds4).
//
// DS4 is an Apple-Silicon MoE inference server. The canonical fixture server
// is vidar:1236. Tested endpoints at time of writing:
//
//   GET /v1/models  — OpenAI-compatible models list (confirmed returns model id)
//   GET /props      — llama.cpp-compatible props (TODO: confirm on live ds4 server)
//   GET /health     — health check (TODO: confirm response shape)
//   GET /v1/system_info — (TODO: unverified; may not exist on ds4)
//
// The extractor tries /props first (richest if supported), then falls back to
// /v1/models. MTP is detected from /props if the server exposes it.
//
// TODO(fizeau-c12e6241): Verify /props and /health endpoint shapes against the
// live vidar:1236 ds4 server. If /props is not supported, remove that branch.
// Also verify whether ds4 exposes MTP status in its API response.

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
	// TODO: confirm this endpoint exists on the live ds4 server at vidar:1236.
	propsURL := baseNoV1 + "/props"
	if raw, err := fetchJSON(ctx, propsURL); err == nil {
		rawData["props"] = raw
		// Parse sampling defaults and context if present.
		var props struct {
			DefaultGenerationSettings struct {
				NCtx        int     `json:"n_ctx"`
				Temperature float64 `json:"temperature"`
				TopP        float64 `json:"top_p"`
				TopK        int     `json:"top_k"`
			} `json:"default_generation_settings"`
			BuildInfo string `json:"build_info"`
			// DS4 may expose MTP state here — TODO: verify field name.
			MTPEnabled *bool `json:"mtp_enabled,omitempty"`
		}
		if b, _ := json.Marshal(raw); b != nil {
			_ = json.Unmarshal(b, &props)
		}
		if props.BuildInfo != "" {
			p.BuildInfo = props.BuildInfo
		}
		if props.DefaultGenerationSettings.NCtx > 0 {
			p.MaxContext = ptrInt(props.DefaultGenerationSettings.NCtx)
		}
		if props.MTPEnabled != nil {
			p.MTPEnabled = props.MTPEnabled
			if *props.MTPEnabled {
				p.DraftMode = "mtp"
			}
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
