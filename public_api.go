package fizeau

import (
	agentcore "github.com/DocumentDrivenDX/fizeau/internal/core"
	"github.com/DocumentDrivenDX/fizeau/internal/modelcatalog"
	"github.com/DocumentDrivenDX/fizeau/internal/reasoning"
)

type Tool = agentcore.Tool

type Reasoning = reasoning.Reasoning
type BillingModel = modelcatalog.BillingModel

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

const (
	BillingModelUnknown      = modelcatalog.BillingModelUnknown
	BillingModelFixed        = modelcatalog.BillingModelFixed
	BillingModelPerToken     = modelcatalog.BillingModelPerToken
	BillingModelSubscription = modelcatalog.BillingModelSubscription
)

func ReasoningTokens(n int) Reasoning {
	return reasoning.ReasoningTokens(n)
}
