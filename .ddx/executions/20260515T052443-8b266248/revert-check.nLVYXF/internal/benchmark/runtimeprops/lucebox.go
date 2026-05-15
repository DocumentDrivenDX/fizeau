package runtimeprops

// lucebox.go — extractor for lucebox-hub (DFlash speculative decoder).
//
// Lucebox serves at sindri:1236/v1. It uses a llama.cpp-compatible API
// surface for the base model (GGUF) with a DFlash speculative draft model.
//
// Tested/confirmed endpoints:
//   GET /v1/models   — OpenAI-compatible model list (confirmed)
//
// Candidate endpoints (TODO: verify on live server):
//   GET /props       — llama.cpp-compatible; may be present since lucebox
//                      is built on llama.cpp internals.
//   GET /v1/server_info — unknown; may not exist.
//
// DFlash speculative decoding is identified by the draft model alias pattern.
// When detected we set draft_mode="dflash".
//
// TODO(fizeau-c12e6241): Verify /props availability against live sindri:1236
// lucebox server. If present, merge the same llama.cpp props parsing used in
// llamacpp.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

func extractLucebox(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	base := strings.TrimRight(lane.BaseURL, "/")
	baseNoV1 := baseURLWithoutV1(base)

	now := time.Now().UTC()
	p := evidence.RuntimeProps{
		Extractor:   "lucebox",
		ExtractedAt: &now,
		BaseModel:   lane.Model,
	}

	rawData := map[string]any{}

	// Try /props (llama.cpp-compatible, may be present).
	// TODO: confirm availability on live sindri:1236 lucebox server.
	propsURL := baseNoV1 + "/props"
	if raw, err := fetchJSON(ctx, propsURL); err == nil {
		rawData["props"] = raw
		var props struct {
			DefaultGenerationSettings struct {
				NCtx int `json:"n_ctx"`
			} `json:"default_generation_settings"`
			BuildInfo string `json:"build_info"`
		}
		if b, _ := json.Marshal(raw); b != nil {
			_ = json.Unmarshal(b, &props)
		}
		if props.DefaultGenerationSettings.NCtx > 0 {
			p.MaxContext = ptrInt(props.DefaultGenerationSettings.NCtx)
		}
		if props.BuildInfo != "" {
			p.BuildInfo = props.BuildInfo
		}
	}

	// /v1/models for the model alias and draft model detection.
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
		if len(models.Data) > 0 {
			p.BaseModel = models.Data[0].ID
		}
	}

	// DFlash is identified by the lane model alias convention "luce-dflash".
	if strings.Contains(strings.ToLower(lane.Model), "dflash") ||
		strings.Contains(strings.ToLower(lane.Model), "luce-dflash") {
		p.DraftMode = "dflash"
	}

	if len(rawData) == 0 {
		return evidence.RuntimeProps{}, fmt.Errorf("lucebox: no endpoints responded at %s", baseNoV1)
	}

	p.PlatformRaw = rawData
	return p, nil
}
