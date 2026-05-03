// Package anthropic implements a agent.Provider for the Anthropic Claude API.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	agent "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/quotaheaders"
	"github.com/DocumentDrivenDX/fizeau/internal/provider/registry"
	ant "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func init() {
	registry.Register(registry.Descriptor{
		Type: "anthropic",
		Factory: func(in registry.Inputs) agent.Provider {
			return New(Config{
				BaseURL:             in.BaseURL,
				APIKey:              in.APIKey,
				Model:               in.Model,
				QuotaSignalObserver: in.QuotaSignalObserver,
			})
		},
		// Anthropic uses api.anthropic.com — no LAN-port inference,
		// explicit BaseURL (or SDK default) required.
	})
}

// Provider implements agent.Provider for the Anthropic Messages API.
type Provider struct {
	client         *ant.Client
	model          string
	providerName   string
	providerSystem string
	serverAddress  string
	serverPort     int
}

// Config holds configuration for the Anthropic provider.
type Config struct {
	APIKey  string
	Model   string // e.g., "claude-sonnet-4-20250514"
	BaseURL string
	// QuotaSignalObserver, when set, receives a parsed Anthropic rate-limit
	// signal on every HTTP response. The service layer wires this to the
	// provider quota state machine so subscription/daily exhaustion routes
	// dispatch around the provider until its reset window elapses.
	QuotaSignalObserver func(quotaheaders.Signal)
}

// New creates a new Anthropic provider.
func New(cfg Config) *Provider {
	opts := []option.RequestOption{option.WithMaxRetries(0)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.QuotaSignalObserver != nil {
		observer := cfg.QuotaSignalObserver
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			resp, err := next(req)
			if resp != nil {
				signal := quotaheaders.ParseAnthropic(resp.Header, time.Now())
				if signal.Present {
					observer(signal)
				}
			}
			return resp, err
		}))
	}
	client := ant.NewClient(opts...)
	serverAddress, serverPort := anthropicIdentity(cfg.BaseURL)
	return &Provider{
		client:         &client,
		model:          cfg.Model,
		providerName:   "anthropic",
		providerSystem: "anthropic",
		serverAddress:  serverAddress,
		serverPort:     serverPort,
	}
}

func (p *Provider) Chat(ctx context.Context, messages []agent.Message, tools []agent.ToolDef, opts agent.Options) (agent.Response, error) {
	model := p.model
	if opts.Model != "" {
		model = opts.Model
	}

	system, convMsgs := buildSystemBlocks(messages, opts)

	params := ant.MessageNewParams{
		Model:    ant.Model(model),
		Messages: convertMessages(convMsgs),
	}

	if len(system) > 0 {
		params.System = system
	}

	maxTokens := 4096
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	params.MaxTokens = int64(maxTokens)

	if opts.Temperature != nil {
		params.Temperature = ant.Float(*opts.Temperature)
	}

	if len(tools) > 0 {
		params.Tools = convertTools(tools, opts)
	}

	var resp agent.Response
	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return resp, fmt.Errorf("anthropic: %w", err)
	}

	resp.Model = string(msg.Model)
	resp.Attempt = &agent.AttemptMetadata{
		ProviderName:   p.providerName,
		ProviderSystem: p.providerSystem,
		ServerAddress:  p.serverAddress,
		ServerPort:     p.serverPort,
		RequestedModel: model,
		ResponseModel:  string(msg.Model),
		ResolvedModel:  string(msg.Model),
		Cost: &agent.CostAttribution{
			Source: agent.CostSourceUnknown,
		},
	}
	resp.Usage = agent.TokenUsage{
		Input:  int(msg.Usage.InputTokens),
		Output: int(msg.Usage.OutputTokens),
		Total:  int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
	}

	// Extract cache tokens if present
	if msg.Usage.CacheCreationInputTokens > 0 {
		resp.Usage.CacheWrite = int(msg.Usage.CacheCreationInputTokens)
	}
	if msg.Usage.CacheReadInputTokens > 0 {
		resp.Usage.CacheRead = int(msg.Usage.CacheReadInputTokens)
	}

	resp.FinishReason = string(msg.StopReason)

	// Extract content and tool calls from content blocks
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			resp.ToolCalls = append(resp.ToolCalls, agent.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: json.RawMessage(block.Input),
			})
		}
	}

	return resp, nil
}

// SessionStartMetadata reports the broad provider identity and configured model
// that should be recorded on session.start events.
func (p *Provider) SessionStartMetadata() (string, string) {
	return p.providerName, p.model
}

// ChatStartMetadata reports the resolved provider system and upstream server
// identity known when the provider is constructed.
func (p *Provider) ChatStartMetadata() (string, string, int) {
	return p.providerSystem, p.serverAddress, p.serverPort
}

func convertMessages(msgs []agent.Message) []ant.MessageParam {
	var result []ant.MessageParam
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			result = append(result, ant.NewUserMessage(ant.NewTextBlock(m.Content)))
		case agent.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				var blocks []ant.ContentBlockParamUnion
				if m.Content != "" {
					blocks = append(blocks, ant.NewTextBlock(m.Content))
				}
				for _, tc := range m.ToolCalls {
					var input interface{}
					_ = json.Unmarshal(tc.Arguments, &input)
					blocks = append(blocks, ant.NewToolUseBlock(tc.ID, input, tc.Name))
				}
				result = append(result, ant.NewAssistantMessage(blocks...))
			} else {
				result = append(result, ant.NewAssistantMessage(ant.NewTextBlock(m.Content)))
			}
		case agent.RoleTool:
			result = append(result, ant.NewUserMessage(
				ant.NewToolResultBlock(m.ToolCallID, m.Content, false),
			))
		}
	}
	return result
}

func convertTools(tools []agent.ToolDef, opts agent.Options) []ant.ToolUnionParam {
	var result []ant.ToolUnionParam
	for _, t := range tools {
		var schema ant.ToolInputSchemaParam
		_ = json.Unmarshal(t.Parameters, &schema)

		result = append(result, ant.ToolUnionParam{
			OfTool: &ant.ToolParam{
				Name:        t.Name,
				Description: ant.String(t.Description),
				InputSchema: schema,
			},
		})
	}
	// Stamp an ephemeral cache_control breakpoint at the END of the tool list
	// so the entire tool prefix becomes a cacheable boundary. CachePolicy "off"
	// suppresses the marker.
	if len(result) > 0 && opts.CachePolicy != "off" {
		last := len(result) - 1
		if result[last].OfTool != nil {
			result[last].OfTool.CacheControl = ant.NewCacheControlEphemeralParam()
		}
	}
	return result
}

// buildSystemBlocks separates system messages from the conversation tail and
// returns the typed Anthropic system-block slice plus the remaining messages.
// When at least one system block is produced and CachePolicy is not "off",
// a cache_control: ephemeral breakpoint is set on the LAST block so the
// system prefix is cached.
func buildSystemBlocks(msgs []agent.Message, opts agent.Options) ([]ant.TextBlockParam, []agent.Message) {
	var system []ant.TextBlockParam
	var convMsgs []agent.Message
	for _, m := range msgs {
		if m.Role == agent.RoleSystem {
			system = append(system, ant.TextBlockParam{Text: m.Content})
		} else {
			convMsgs = append(convMsgs, m)
		}
	}
	if len(system) > 0 && opts.CachePolicy != "off" {
		system[len(system)-1].CacheControl = ant.NewCacheControlEphemeralParam()
	}
	return system, convMsgs
}

// ChatStream implements agent.StreamingProvider for token-level streaming.
func (p *Provider) ChatStream(ctx context.Context, messages []agent.Message, tools []agent.ToolDef, opts agent.Options) (<-chan agent.StreamDelta, error) {
	model := p.model
	if opts.Model != "" {
		model = opts.Model
	}

	system, convMsgs := buildSystemBlocks(messages, opts)

	params := ant.MessageNewParams{
		Model:    ant.Model(model),
		Messages: convertMessages(convMsgs),
	}
	if len(system) > 0 {
		params.System = system
	}
	maxTokens := 4096
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	params.MaxTokens = int64(maxTokens)
	if opts.Temperature != nil {
		params.Temperature = ant.Float(*opts.Temperature)
	}
	if len(tools) > 0 {
		params.Tools = convertTools(tools, opts)
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	ch := make(chan agent.StreamDelta, 1)
	go func() {
		defer close(ch)
		send := func(delta agent.StreamDelta) {
			delta.ArrivedAt = time.Now()
			ch <- delta
		}

		// Track current tool use block being streamed
		var currentToolID string
		var currentToolName string
		responseModel := model

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "message_start":
				// Capture input tokens from message_start
				if event.Message.Model != "" {
					responseModel = string(event.Message.Model)
				}
				send(agent.StreamDelta{
					Model: responseModel,
					Usage: &agent.TokenUsage{
						Input: int(event.Usage.InputTokens),
					},
				})

			case "content_block_start":
				if event.ContentBlock.Type == "tool_use" {
					currentToolID = event.ContentBlock.ID
					currentToolName = event.ContentBlock.Name
					send(agent.StreamDelta{
						ToolCallID:   currentToolID,
						ToolCallName: currentToolName,
					})
				}

			case "content_block_delta":
				// Text delta
				if event.Delta.Text != "" {
					send(agent.StreamDelta{Content: event.Delta.Text})
				}
				// Tool input JSON delta
				if event.Delta.PartialJSON != "" {
					send(agent.StreamDelta{
						ToolCallID:   currentToolID,
						ToolCallArgs: event.Delta.PartialJSON,
					})
				}

			case "content_block_stop":
				currentToolID = ""
				currentToolName = ""

			case "message_delta":
				delta := agent.StreamDelta{
					FinishReason: string(event.Delta.StopReason),
				}
				// Build usage with output and cache tokens
				usage := &agent.TokenUsage{}
				if event.Usage.OutputTokens > 0 {
					usage.Output = int(event.Usage.OutputTokens)
				}
				if event.Usage.CacheCreationInputTokens > 0 {
					usage.CacheWrite = int(event.Usage.CacheCreationInputTokens)
				}
				if event.Usage.CacheReadInputTokens > 0 {
					usage.CacheRead = int(event.Usage.CacheReadInputTokens)
				}
				if usage.Input > 0 || usage.Output > 0 || usage.CacheRead > 0 || usage.CacheWrite > 0 {
					delta.Usage = usage
				}
				send(delta)

			case "message_stop":
				if err := stream.Err(); err != nil {
					send(agent.StreamDelta{Err: err})
					return
				}
				send(agent.StreamDelta{
					Model: responseModel,
					Attempt: &agent.AttemptMetadata{
						ProviderName:   p.providerName,
						ProviderSystem: p.providerSystem,
						ServerAddress:  p.serverAddress,
						ServerPort:     p.serverPort,
						RequestedModel: model,
						ResponseModel:  responseModel,
						ResolvedModel:  responseModel,
						Cost: &agent.CostAttribution{
							Source: agent.CostSourceUnknown,
						},
					},
					Done: true,
				})
				return
			}
		}

		// Stream ended without message_stop — check for error.
		if err := stream.Err(); err != nil {
			send(agent.StreamDelta{Err: err})
			return
		}
		send(agent.StreamDelta{
			Model: responseModel,
			Attempt: &agent.AttemptMetadata{
				ProviderName:   p.providerName,
				ProviderSystem: p.providerSystem,
				ServerAddress:  p.serverAddress,
				ServerPort:     p.serverPort,
				RequestedModel: model,
				ResponseModel:  responseModel,
				ResolvedModel:  responseModel,
				Cost: &agent.CostAttribution{
					Source: agent.CostSourceUnknown,
				},
			},
			Done: true,
		})
	}()

	return ch, nil
}

var _ agent.Provider = (*Provider)(nil)
var _ agent.StreamingProvider = (*Provider)(nil)

func anthropicIdentity(baseURL string) (serverAddress string, serverPort int) {
	serverAddress = "api.anthropic.com"
	serverPort = 443

	if baseURL == "" {
		return serverAddress, serverPort
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return serverAddress, serverPort
	}

	if host := parsed.Hostname(); host != "" {
		serverAddress = host
	}

	if port := parsed.Port(); port != "" {
		if parsedPort, err := strconv.Atoi(port); err == nil {
			serverPort = parsedPort
		}
	} else if strings.EqualFold(parsed.Scheme, "http") {
		serverPort = 80
	}

	return serverAddress, serverPort
}
