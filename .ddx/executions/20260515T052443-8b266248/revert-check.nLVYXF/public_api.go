package fizeau

import (
	agentcore "github.com/easel/fizeau/internal/core"
	"github.com/easel/fizeau/internal/harnesses"
	"github.com/easel/fizeau/internal/modelcatalog"
	"github.com/easel/fizeau/internal/reasoning"
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

// HarnessAvailability reports the install status of a single harness CLI as
// observed by AvailableHarnesses / DetectHarnesses. It is a thin, read-only
// snapshot — the richer ListHarnesses(ctx) surface on FizeauService remains
// the source of truth for live status (quota, account, capability matrix).
type HarnessAvailability struct {
	// Name is the canonical harness identifier (e.g. "claude", "codex").
	Name string
	// Binary is the executable name probed on PATH (e.g. "claude").
	// Empty for HTTP-only providers and embedded harnesses.
	Binary string
	// Available is true when the binary resolved on PATH, when the harness
	// is embedded (always available), or when it is an HTTP-only provider
	// (availability depends on a probe, surfaced via ListHarnesses).
	Available bool
	// Path is the resolved binary location, "(embedded)" for in-process
	// harnesses, or "(http)" for HTTP-only providers.
	Path string
	// Error is non-empty when Available is false; typically "binary not found".
	Error string
}

// AvailableHarnesses returns the canonical names of every harness whose CLI
// is detected on the user's PATH at call time, plus embedded harnesses
// (always available) and HTTP-only providers (availability decided by a
// probe). Names are returned in the registry preference order so the first
// entry is the highest-priority installed wrapper.
//
// This function is intentionally cheap — it only invokes exec.LookPath for
// each builtin harness binary and never spawns a subprocess. Callers who
// need richer state (quota, auth, capability matrix) should construct a
// FizeauService and call ListHarnesses.
func AvailableHarnesses() []string {
	statuses := harnesses.NewRegistry().Discover()
	out := make([]string, 0, len(statuses))
	for _, s := range statuses {
		if s.Available {
			out = append(out, s.Name)
		}
	}
	return out
}

// DetectHarnesses returns the install snapshot for every builtin harness in
// preference order, including unavailable ones. Use AvailableHarnesses when
// only the installed names are needed.
func DetectHarnesses() []HarnessAvailability {
	statuses := harnesses.NewRegistry().Discover()
	out := make([]HarnessAvailability, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, HarnessAvailability{
			Name:      s.Name,
			Binary:    s.Binary,
			Available: s.Available,
			Path:      s.Path,
			Error:     s.Error,
		})
	}
	return out
}
