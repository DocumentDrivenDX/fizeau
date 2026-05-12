package openai

// Protocol-level capability flags describe what the concrete provider can
// honor on the OpenAI-compatible wire: tool calls, streaming, and structured
// output modes. They are distinct from routing-layer capability (a
// benchmark-quality score used by smart routing). Callers use these flags to
// gate dispatch before sending unsupported request shapes.

// ProtocolCapabilities declares provider-owned protocol capability claims.
type ProtocolCapabilities struct {
	Tools            bool
	Stream           bool
	StructuredOutput bool
	// Thinking reports whether the provider accepts non-standard body fields
	// (openai-go WithJSONSet) used to control model-side reasoning.
	Thinking       bool
	ThinkingFormat ThinkingWireFormat
	// StrictThinkingModelMatch, when true, makes the openai layer return an
	// error if the request carries an explicit reasoning policy while the
	// model does not match the provider's wire format family (e.g. a Qwen
	// wire format with a non-Qwen model). Set true for providers that only
	// serve a single model family (OMLX → Qwen MLX). Providers that host
	// mixed model families (LM Studio can load Qwen, Gemma, Llama, etc.)
	// should leave this false so reasoning controls silently no-op on
	// non-matching models instead of failing the request.
	StrictThinkingModelMatch bool
	// ImplicitGenerationConfig declares that the inference server applies
	// the model's HuggingFace `generation_config.json` automatically when
	// the request omits sampler fields. vLLM does this by default; most
	// other local servers (omlx, lmstudio, lucebox) ship custom presets and
	// either ignore upstream defaults or replace them at repackage time
	// (MLX / GGUF strip generation_config.json), which is why ADR-007's
	// catalog sampling_profiles exist. Routing and the catalog-stale
	// nudge use this flag to distinguish "server has a sane default" from
	// "server will decode greedy" when no catalog profile is supplied.
	ImplicitGenerationConfig bool
	// ReasoningAliasMap carries the L1 alias map from live introspection.
	// Keys are caller-visible tiers and values are canonical tiers.
	ReasoningAliasMap map[string]string
	// EffectiveReasoningLevels carries the L1 canonical reasoning tiers after
	// alias de-duplication.
	EffectiveReasoningLevels []string
	// SupportedRequestParams carries the L1 supported-request-parameter list
	// from live introspection. The translator uses it as an additional hint
	// when the explicit ThinkingFormat is absent.
	SupportedRequestParams []string
	// ServerSideReasoningFormat carries the server's default reasoning_format
	// value when the provider reports one through live introspection.
	ServerSideReasoningFormat string
}

type ThinkingWireFormat string

const (
	// ThinkingWireFormatThinkingMap sends `thinking: {type, budget_tokens}`.
	ThinkingWireFormatThinkingMap ThinkingWireFormat = "thinking_map"
	// ThinkingWireFormatQwen sends Qwen-family controls:
	// `enable_thinking` and `thinking_budget`.
	ThinkingWireFormatQwen ThinkingWireFormat = "qwen"
	// ThinkingWireFormatOpenRouter sends OpenRouter's nested `reasoning`
	// object with `effort`, `max_tokens`, or `exclude`.
	ThinkingWireFormatOpenRouter ThinkingWireFormat = "openrouter"
	// ThinkingWireFormatOpenAIEffort sends flat top-level
	// `reasoning_effort: "<tier>"`. Disable form: top-level `think: false`.
	// Used by ds4 / deepseek-v4-flash.
	ThinkingWireFormatOpenAIEffort ThinkingWireFormat = "openai_effort"
)

var (
	OpenAIProtocolCapabilities  = ProtocolCapabilities{Tools: true, Stream: true, StructuredOutput: true, Thinking: false}
	UnknownProtocolCapabilities ProtocolCapabilities
)

func (p *Provider) protocolCapabilities() ProtocolCapabilities {
	if p.capabilities != nil {
		return *p.capabilities
	}
	return OpenAIProtocolCapabilities
}

// SupportsTools reports whether the concrete provider accepts a `tools`
// field on `/v1/chat/completions` and returns structured `tool_calls` in the
// response.
func (p *Provider) SupportsTools() bool {
	return p.protocolCapabilities().Tools
}

// SupportsStream reports whether `stream: true` returns a well-formed SSE
// stream with incremental `choices[0].delta` chunks.
func (p *Provider) SupportsStream() bool {
	return p.protocolCapabilities().Stream
}

// SupportsStructuredOutput reports whether the provider honors
// `response_format: json_object` / tool-use-required semantics to produce a
// structured (JSON-shaped) response.
func (p *Provider) SupportsStructuredOutput() bool {
	return p.protocolCapabilities().StructuredOutput
}

// SupportsThinking reports whether the provider accepts non-standard request
// body fields used to cap or disable model-side reasoning. Providers returning
// false MUST have those fields stripped at serialization time.
func (p *Provider) SupportsThinking() bool {
	return p.protocolCapabilities().Thinking
}

func (p *Provider) thinkingWireFormat() ThinkingWireFormat {
	caps := p.protocolCapabilities()
	if !caps.Thinking {
		return ""
	}
	if caps.ThinkingFormat != "" {
		return caps.ThinkingFormat
	}
	if hasSupportedRequestParam(caps.SupportedRequestParams, "reasoning_effort") ||
		hasSupportedRequestParam(caps.SupportedRequestParams, "think") {
		return ThinkingWireFormatOpenAIEffort
	}
	if hasSupportedRequestParam(caps.SupportedRequestParams, "enable_thinking") ||
		hasSupportedRequestParam(caps.SupportedRequestParams, "thinking_budget") {
		return ThinkingWireFormatQwen
	}
	if hasSupportedRequestParam(caps.SupportedRequestParams, "budget_tokens") ||
		hasSupportedRequestParam(caps.SupportedRequestParams, "thinking") {
		return ThinkingWireFormatThinkingMap
	}
	return ThinkingWireFormatThinkingMap
}

func hasSupportedRequestParam(params []string, target string) bool {
	for _, param := range params {
		if param == target {
			return true
		}
	}
	return false
}

func (p *Provider) strictThinkingModelMatch() bool {
	return p.protocolCapabilities().StrictThinkingModelMatch
}

// ImplicitGenerationConfig reports whether the provider's server applies a
// model-card-derived sampler bundle (`generation_config.json`) when the
// request omits sampler fields. Used by the agent CLI to tone the
// catalog-stale nudge per ADR-007 §7.
func (p *Provider) ImplicitGenerationConfig() bool {
	return p.protocolCapabilities().ImplicitGenerationConfig
}
