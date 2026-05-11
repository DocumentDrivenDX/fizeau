// probe-reasoning sends a controlled prompt to each Qwen lane at multiple
// reasoning levels and reports per-response token usage. The hypothesis we're
// testing: setting reasoning=low/medium/high through fizeau actually changes
// the upstream thinking_budget the model honors, observable as a roughly
// monotonic increase in output_tokens.
//
// If the budget is being honored, output_tokens should rise materially with
// the reasoning tier. If it's being ignored, output_tokens stays roughly
// constant across tiers (the model thinks as much as it wants regardless).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/provider/ds4"
	"github.com/easel/fizeau/internal/provider/llamaserver"
	"github.com/easel/fizeau/internal/provider/omlx"
	"github.com/easel/fizeau/internal/provider/openrouter"
)

type lane struct {
	name       string
	build      func() agentcore.Provider
}

const prompt = "Solve this step by step: A train leaves Boston at 9am traveling 60mph east. " +
	"Another leaves NYC at 10am traveling 50mph north. " +
	"At noon, how far apart are they if NYC is 200 miles SW of Boston? " +
	"Show your work and give a final numeric answer in miles."

func main() {
	tiers := []agentcore.Reasoning{"off", "low", "medium", "high"}
	orKey := os.Getenv("OPENROUTER_API_KEY")
	if orKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY not set — skipping OR lane")
	}

	lanes := []lane{
		{name: "openrouter-qwen3.6-27b", build: func() agentcore.Provider {
			return openrouter.New(openrouter.Config{
				BaseURL: "https://openrouter.ai/api/v1",
				APIKey:  orKey,
				Model:   "qwen/qwen3.6-27b",
			})
		}},
		{name: "sindri-llamacpp", build: func() agentcore.Provider {
			return llamaserver.New(llamaserver.Config{
				BaseURL: "http://sindri:8020/v1",
				APIKey:  "ignore",
				Model:   "Qwen3.6-27B-UD-Q3_K_XL.gguf",
			})
		}},
		{name: "vidar-omlx", build: func() agentcore.Provider {
			return omlx.New(omlx.Config{
				BaseURL: "http://vidar:1235/v1",
				APIKey:  "ignore",
				Model:   "Qwen3.6-27B-MLX-8bit",
			})
		}},
		{name: "vidar-ds4-deepseek", build: func() agentcore.Provider {
			return ds4.New(ds4.Config{
				BaseURL: "http://vidar:1236/v1",
				APIKey:  "ignore",
				Model:   "deepseek-v4-flash",
			})
		}},
	}

	fmt.Printf("%-30s  %-7s  %6s  %6s  %6s  %6s  %s\n",
		"lane", "tier", "in_tok", "out_tok", "wall_s", "stop", "first_60_chars")

	for _, lane := range lanes {
		if lane.name == "openrouter-qwen3.6-27b" && orKey == "" {
			continue
		}
		p := lane.build()
		for _, tier := range tiers {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			start := time.Now()
			resp, err := p.Chat(ctx, []agentcore.Message{
				{Role: agentcore.RoleUser, Content: prompt},
			}, nil, agentcore.Options{Reasoning: tier})
			cancel()
			elapsed := time.Since(start).Seconds()
			if err != nil {
				fmt.Printf("%-30s  %-7s  ERR: %v\n", lane.name, string(tier), err)
				continue
			}
			content := resp.Content
			if len(content) > 60 {
				content = content[:60]
			}
			content = sanitize(content)
			fmt.Printf("%-30s  %-7s  %6d  %6d  %6.1f  %-6s  %s\n",
				lane.name, string(tier),
				resp.Usage.Input, resp.Usage.Output, elapsed,
				resp.FinishReason, content)
		}
		fmt.Println()
	}
}

func sanitize(s string) string {
	out := []rune{}
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			out = append(out, ' ')
		} else if r >= 32 && r < 127 {
			out = append(out, r)
		}
	}
	return string(out)
}

var _ = json.Marshal // silence unused import in case build mode changes
