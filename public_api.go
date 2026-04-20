package agent

import (
	agentcore "github.com/DocumentDrivenDX/agent/internal/core"
	"github.com/DocumentDrivenDX/agent/internal/reasoning"
)

type Tool = agentcore.Tool

type Reasoning = reasoning.Reasoning

const (
	ReasoningAuto    = reasoning.ReasoningAuto
	ReasoningOff     = reasoning.ReasoningOff
	ReasoningLow     = reasoning.ReasoningLow
	ReasoningMedium  = reasoning.ReasoningMedium
	ReasoningHigh    = reasoning.ReasoningHigh
	ReasoningMinimal = reasoning.ReasoningMinimal
	ReasoningXHigh   = reasoning.ReasoningXHigh
	ReasoningMax     = reasoning.ReasoningMax
)

func ReasoningTokens(n int) Reasoning {
	return reasoning.ReasoningTokens(n)
}
