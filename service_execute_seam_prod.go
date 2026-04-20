//go:build !testseam

package agent

// Production builds have no test seams. Each helper returns nil so
// service.Execute compiles identically to the testseam variant but the
// hooks never fire. Production cannot reference FakeProvider or any of
// the assertion hooks because seamOptions is empty (options_prod.go).

func (s *service) promptAssertionHook() promptAssertionHookFn         { return nil }
func (s *service) compactionAssertionHook() compactionAssertionHookFn { return nil }
func (s *service) toolWiringHook() toolWiringHookFn                   { return nil }

func (s *service) resolveNativeProvider(req ServiceExecuteRequest) nativeProviderResolution {
	return s.resolveConfiguredNativeProvider(req)
}

// promptAssertionHookFn / compactionAssertionHookFn / toolWiringHookFn
// are the function-typed aliases used by the helper signatures above so
// service_execute.go compiles without referencing the testseam-only types
// directly. Their definitions are identical across builds.
type promptAssertionHookFn func(systemPrompt, userPrompt string, contextFiles []string)
type compactionAssertionHookFn func(messagesBefore, messagesAfter int, tokensFreed int)
type toolWiringHookFn func(harness string, toolNames []string)
