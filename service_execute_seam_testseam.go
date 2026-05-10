//go:build testseam

package fizeau

import (
	"context"

	agentcore "github.com/easel/fizeau/internal/core"
)

// Test builds expose the four CONTRACT-003 seams via embedded seamOptions.
// Each helper returns the configured hook (possibly nil) and the native
// provider resolver consults FakeProvider to bypass real HTTP.

// promptAssertionHookFn / compactionAssertionHookFn / toolWiringHookFn
// are the function-typed aliases shared by service_execute.go. In test
// builds they alias the seam types from testseam_types.go.
type promptAssertionHookFn = PromptAssertionHook
type compactionAssertionHookFn = CompactionAssertionHook
type toolWiringHookFn = ToolWiringHook

func (s *service) promptAssertionHook() promptAssertionHookFn {
	return s.opts.PromptAssertionHook
}

func (s *service) compactionAssertionHook() compactionAssertionHookFn {
	return s.opts.CompactionAssertionHook
}

func (s *service) toolWiringHook() toolWiringHookFn {
	return s.opts.ToolWiringHook
}

func (s *service) resolveNativeProvider(req ServiceExecuteRequest) nativeProviderResolution {
	if s.opts.FakeProvider != nil {
		return nativeProviderResolution{
			Provider: &fakeProviderAdapter{fp: s.opts.FakeProvider},
			Name:     req.Provider,
			Entry:    ServiceProviderEntry{Model: req.Model},
		}
	}
	return s.resolveConfiguredNativeProvider(req)
}

// fakeProviderAdapter wraps a *FakeProvider so it satisfies the
// core.Provider interface. Static responses are consumed in order;
// Dynamic is invoked per call when set; InjectError fires per-call to
// optionally return an error before the response.
type fakeProviderAdapter struct {
	fp        *FakeProvider
	callIndex int
}

func (a *fakeProviderAdapter) Chat(ctx context.Context, messages []agentcore.Message, tools []agentcore.ToolDef, opts agentcore.Options) (agentcore.Response, error) {
	defer func() { a.callIndex++ }()
	if a.fp == nil {
		return agentcore.Response{}, nil
	}
	if a.fp.InjectError != nil {
		if err := a.fp.InjectError(a.callIndex); err != nil {
			return agentcore.Response{}, err
		}
	}
	if a.fp.Dynamic != nil {
		toolNames := make([]string, len(tools))
		for i, t := range tools {
			toolNames[i] = t.Name
		}
		freq := FakeRequest{
			Messages:    messages,
			Tools:       toolNames,
			Model:       opts.Model,
			Temperature: opts.Temperature,
			Seed:        opts.Seed,
			Reasoning:   opts.Reasoning,
		}
		fresp, err := a.fp.Dynamic(freq)
		if err != nil {
			return agentcore.Response{}, err
		}
		return fakeResponseToResponse(fresp), nil
	}
	if a.callIndex < len(a.fp.Static) {
		return fakeResponseToResponse(a.fp.Static[a.callIndex]), nil
	}
	// Out of static script — return an empty response so the loop
	// terminates with no further tool calls.
	return agentcore.Response{Content: "", Usage: agentcore.TokenUsage{}}, nil
}

func fakeResponseToResponse(fr FakeResponse) agentcore.Response {
	return agentcore.Response{
		Content:   fr.Text,
		ToolCalls: fr.ToolCalls,
		Usage:     fr.Usage,
	}
}
