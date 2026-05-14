package runtimeprops

// rapid_mlx.go — extractor for Rapid-MLX (Apple Silicon MLX server, grendel).
//
// Rapid-MLX exposes an OpenAI-compatible /v1/models endpoint.
// Confirmed: /health returns ready at grendel:8000.
// Model list at /v1/models lists mlx-community/Qwen3.6-27B-8bit.
//
// TODO(fizeau-c12e6241): Verify if Rapid-MLX exposes quantization or context
// length metadata beyond the model id in /v1/models. Check /health response
// for any additional metadata fields.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

func extractRapidMLX(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	base := strings.TrimRight(lane.BaseURL, "/")
	baseNoV1 := baseURLWithoutV1(base)

	now := time.Now().UTC()
	p := evidence.RuntimeProps{
		Extractor:   "rapid-mlx",
		ExtractedAt: &now,
		BaseModel:   lane.Model,
	}

	modelsURL := baseNoV1 + "/v1/models"
	raw, err := fetchJSON(ctx, modelsURL)
	if err != nil {
		return evidence.RuntimeProps{}, wrapFetchError("rapid-mlx", modelsURL, err)
	}

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

	p.PlatformRaw = raw
	return p, nil
}
