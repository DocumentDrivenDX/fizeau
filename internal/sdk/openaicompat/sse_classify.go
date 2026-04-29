package openaicompat

import (
	"regexp"
	"strings"

	agent "github.com/DocumentDrivenDX/agent/internal/core"
)

// notImplementedCapabilityPattern extracts the capability name from a server
// error string of the form "...NotImplementedError: <Capability> NYI..." or
// "...NotImplementedError: <Capability>". The trailing " NYI" sentinel is
// what mlx_lm uses; we strip it from the captured group when present.
var notImplementedCapabilityPattern = regexp.MustCompile(`NotImplementedError:\s*([^"\\]+?)(?:\s+NYI)?(?:["\\]|$)`)

// classifyStreamErr inspects an error returned from the openai-go streaming
// decoder and, when the upstream server reported a not-implemented capability
// (e.g. mlx_lm's "RotatingKVCache Quantization NYI"), returns a typed
// agent.ProviderCapabilityMissingError that wraps the original error. For
// inputs the classifier does not recognize, it returns err unchanged.
//
// The classifier matches on the substring "NotImplementedError" — both the
// specific RotatingKVCache case named in the bead and any other
// not-implemented capability the same server family reports. Substring
// matching is intentional: the openai-go SDK has already serialized the
// upstream error payload into its own message ("received error while
// streaming: <json>"), and the classifier should be robust to surrounding
// formatting it does not control.
func classifyStreamErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if !strings.Contains(msg, "NotImplementedError") {
		return err
	}
	return &agent.ProviderCapabilityMissingError{
		Code:          agent.ProviderCapabilityMissingErrorCode,
		Capability:    extractNotImplementedCapability(msg),
		ServerMessage: msg,
		Cause:         err,
	}
}

func extractNotImplementedCapability(msg string) string {
	match := notImplementedCapabilityPattern.FindStringSubmatch(msg)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
