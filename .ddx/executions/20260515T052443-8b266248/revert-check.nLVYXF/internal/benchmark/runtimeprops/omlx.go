package runtimeprops

// omlx.go — extractor for oMLX (Apple Silicon MLX inference server).
//
// oMLX exposes an OpenAI-compatible /v1/models endpoint. The model list
// includes the loaded model id. MTP is not applicable on oMLX.
// Confirmed endpoint at vidar:1235/v1.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

func extractOMLX(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	base := strings.TrimRight(lane.BaseURL, "/")
	baseNoV1 := baseURLWithoutV1(base)

	now := time.Now().UTC()
	p := evidence.RuntimeProps{
		Extractor:   "omlx",
		ExtractedAt: &now,
		BaseModel:   lane.Model,
	}

	modelsURL := baseNoV1 + "/v1/models"
	raw, err := fetchJSON(ctx, modelsURL)
	if err != nil {
		return evidence.RuntimeProps{}, wrapFetchError("omlx", modelsURL, err)
	}

	var models struct {
		Data []struct {
			ID string `json:"id"`
			// oMLX may include quantization or context info here.
			// TODO: confirm exact field names against a live oMLX server.
			MaxContextLength int    `json:"max_context_length,omitempty"`
			Quantization     string `json:"quantization,omitempty"`
		} `json:"data"`
	}
	if b, _ := json.Marshal(raw); b != nil {
		_ = json.Unmarshal(b, &models)
	}
	if len(models.Data) > 0 {
		p.BaseModel = models.Data[0].ID
		if models.Data[0].MaxContextLength > 0 {
			p.MaxContext = ptrInt(models.Data[0].MaxContextLength)
		}
		if models.Data[0].Quantization != "" {
			p.ModelQuant = models.Data[0].Quantization
		}
	}

	p.PlatformRaw = raw
	return p, nil
}
