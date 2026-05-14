// Package runtimeprops captures per-cell server-reported runtime properties
// from supported inference platforms. Each platform has its own extractor file.
// The dispatcher picks the extractor by the lane's runtime/provider type field.
//
// Caller contract:
//   - Call Extract once per cell, after preflight, before the bench run.
//   - Never fail the cell on extraction error; log and continue.
//   - If extraction fails, the returned Props will have ExtractionFailed set.
package runtimeprops

import (
	"context"
	"fmt"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
)

// LaneInfo holds the minimal lane information needed to pick and run an extractor.
type LaneInfo struct {
	// Runtime is the lane's runtime field (e.g. "llamacpp", "vllm", "ds4",
	// "omlx", "lucebox", "rapid-mlx", "openrouter", "openai"). Matches
	// profile.ProviderType strings.
	Runtime string

	// BaseURL is the inference server base URL, e.g. "http://sindri:8020/v1".
	BaseURL string

	// Model is the model identifier string as sent to the provider.
	Model string
}

// Extract dispatches to the appropriate extractor for the lane's runtime and
// returns the extracted Props. On error it returns a Props with ExtractionFailed
// set plus the error — callers should log but never fail the cell.
func Extract(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error) {
	extractor, ok := dispatchExtractor(lane.Runtime)
	if !ok {
		// Cloud / unknown platform: stamp a minimal cloud record.
		now := time.Now().UTC()
		p := evidence.RuntimeProps{
			Extractor:   "cloud",
			ExtractedAt: &now,
			BaseModel:   lane.Model,
		}
		return p, nil
	}
	props, err := extractor(ctx, lane)
	if err != nil {
		now := time.Now().UTC()
		return evidence.RuntimeProps{
			Extractor:        lane.Runtime,
			ExtractedAt:      &now,
			ExtractionFailed: err.Error(),
		}, err
	}
	return props, nil
}

type extractorFn func(ctx context.Context, lane LaneInfo) (evidence.RuntimeProps, error)

func dispatchExtractor(runtime string) (extractorFn, bool) {
	switch runtime {
	case "llama-server", "llamacpp", "llama.cpp":
		return extractLlamaCPP, true
	case "vllm":
		return extractVLLM, true
	case "ds4":
		return extractDS4, true
	case "omlx":
		return extractOMLX, true
	case "lucebox", "lucebox-dflash":
		return extractLucebox, true
	case "rapid-mlx":
		return extractRapidMLX, true
	case "openrouter", "openai", "anthropic", "google":
		return nil, false // cloud: caller stamps base_model from first response
	default:
		return nil, false
	}
}

// ptrInt is a convenience helper for pointer-to-int values.
func ptrInt(v int) *int { return &v }

// ptrBool is a convenience helper for pointer-to-bool values.
func ptrBool(v bool) *bool { return &v }

// ptrFloat64 is a convenience helper for pointer-to-float64 values.
func ptrFloat64(v float64) *float64 { return &v }

// baseURLWithoutV1 strips a trailing "/v1" from a base URL.
func baseURLWithoutV1(base string) string {
	if len(base) >= 3 && base[len(base)-3:] == "/v1" {
		return base[:len(base)-3]
	}
	return base
}

// fetchTimeout is the per-request timeout for all platform extractors.
const fetchTimeout = 5 * time.Second

// wrapFetchError wraps a fetch error with platform context.
func wrapFetchError(platform, url string, err error) error {
	return fmt.Errorf("%s: fetch %s: %w", platform, url, err)
}
