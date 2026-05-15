//go:build testseam

package fizeau

// seamOptions carries the four test injection seams. It is embedded into
// Options and is only compiled when the testseam build tag is set.
// Production builds get an empty seamOptions from options_prod.go, making
// it a compile error to reference any of these fields without the tag.
type seamOptions struct {
	// FakeProvider replaces the real provider during testing. When non-nil,
	// the agent uses it instead of making real network calls.
	FakeProvider *FakeProvider

	// PromptAssertionHook is called once per Execute with the resolved
	// system+user prompt and context files.
	PromptAssertionHook PromptAssertionHook

	// CompactionAssertionHook is called on every real compaction pass.
	CompactionAssertionHook CompactionAssertionHook

	// ToolWiringHook is called once per Execute with the harness name and
	// the resolved tool list.
	ToolWiringHook ToolWiringHook
}

func shouldAutoLoadServiceConfig(opts ServiceOptions) bool {
	return opts.ConfigPath != ""
}
