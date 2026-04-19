// Package conformance contains shared provider behavior tests.
//
// To register a new provider or provider flavor, add a package-local test that
// builds a shaped double server, returns a Subject from a Factory, and calls
// Run with a Capabilities descriptor. Keep protocol-specific HTTP fixtures in
// the provider package so this package only depends on agent.Provider behavior.
//
// To add a new capability, extend Capabilities and add one subtest in Run. New
// capability checks should be opt-in when real model behavior is unreliable or
// provider-specific.
//
// Live smoke tests should reuse the same Run entry point and skip unless their
// environment is configured. Current live switches:
//
//   - OMLX_URL for omlx OpenAI-compatible endpoints.
//   - LMSTUDIO_URL for LM Studio OpenAI-compatible endpoints.
//   - OPENROUTER_API_KEY for OpenRouter.
//   - OPENAI_API_KEY for OpenAI.
//   - OLLAMA_URL for Ollama.
//   - ANTHROPIC_API_KEY for Anthropic.
package conformance
