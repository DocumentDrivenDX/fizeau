package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/harnesses"
)

// opencodeEnvelope is a minimal view of the opencode --format json output.
// opencode emits a JSON object (may be on a single line or multiple lines)
// with the response text and optional usage fields.
//
// From DDx ExtractUsage: envelope.Usage.InputTokens, envelope.Usage.OutputTokens,
// envelope.TotalCostUSD. Response text is the raw output.
type opencodeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type opencodeEnvelope struct {
	// Pointer so a missing usage object stays nil; presence (even with all-
	// zero counts) signals an upstream-reported usage envelope per
	// CONTRACT-003.
	Usage        *opencodeUsage `json:"usage,omitempty"`
	TotalCostUSD float64        `json:"total_cost_usd"`
}

// streamAggregate captures usage from the opencode output. HasUsage is set
// when the provider envelope carried a usage object — InputTokens /
// OutputTokens then preserve the upstream values verbatim, including zero.
type streamAggregate struct {
	FinalText    string
	HasUsage     bool
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

// parseOpencodeStream reads opencode --format json output from r and emits
// harness Events on out. opencode produces a JSON object after completion
// (buffered, not streaming line-by-line). We buffer all output, emit a single
// text_delta with the content, then parse usage from the JSON envelope.
//
// If the output is valid JSON with a usage envelope, usage is extracted.
// Otherwise the raw output is emitted as-is as a text_delta.
func parseOpencodeStream(ctx context.Context, r io.Reader, out chan<- harnesses.Event, metadata map[string]string, seq *int64) (*streamAggregate, error) {
	agg := &streamAggregate{}

	emit := func(t harnesses.EventType, data any) error {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		ev := harnesses.Event{
			Type:     t,
			Sequence: *seq,
			Time:     time.Now().UTC(),
			Metadata: metadata,
			Data:     raw,
		}
		*seq++
		select {
		case out <- ev:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Buffer all output — opencode produces a JSON block, not JSONL.
	var buf strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return agg, ctx.Err()
		default:
		}
		buf.WriteString(scanner.Text())
		buf.WriteString("\n")
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return agg, err
	}

	output := strings.TrimSpace(buf.String())
	if output == "" {
		return agg, nil
	}
	if errMessage, ok := opencodeErrorMessage(output); ok {
		return agg, errors.New("opencode error: " + errMessage)
	}

	// Try to parse as a JSON envelope to extract usage.
	// opencode may emit usage in the envelope or as the last non-empty line.
	// Detection is by *envelope presence* (Usage pointer non-nil OR cost
	// reported), not by positive values, so explicit upstream zeros are
	// preserved per CONTRACT-003.
	applyEnv := func(env opencodeEnvelope) bool {
		if env.Usage != nil {
			agg.HasUsage = true
			agg.InputTokens = env.Usage.InputTokens
			agg.OutputTokens = env.Usage.OutputTokens
		}
		if env.TotalCostUSD > 0 {
			agg.CostUSD = env.TotalCostUSD
		}
		return env.Usage != nil || env.TotalCostUSD > 0
	}

	parsed := false
	var env opencodeEnvelope
	if err := json.Unmarshal([]byte(output), &env); err == nil {
		if applyEnv(env) {
			parsed = true
		}
	}
	if !parsed {
		// Try last non-empty line.
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			var env2 opencodeEnvelope
			if err := json.Unmarshal([]byte(line), &env2); err == nil {
				applyEnv(env2)
			}
			break
		}
	}

	// Emit raw output as a text_delta — opencode returns clean text.
	agg.FinalText = output
	if err := emit(harnesses.EventTypeTextDelta, harnesses.TextDeltaData{Text: output}); err != nil {
		return agg, err
	}

	return agg, nil
}

func opencodeErrorMessage(output string) (string, bool) {
	var envelope struct {
		Type  string `json:"type"`
		Error struct {
			Name string `json:"name"`
			Data struct {
				Message string `json:"message"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		return "", false
	}
	if envelope.Type != "error" {
		return "", false
	}
	switch {
	case envelope.Error.Data.Message != "":
		return envelope.Error.Data.Message, true
	case envelope.Error.Name != "":
		return envelope.Error.Name, true
	default:
		return "unknown error", true
	}
}
