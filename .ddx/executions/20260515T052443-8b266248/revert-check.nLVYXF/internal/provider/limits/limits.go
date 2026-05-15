package limits

// ModelLimits holds context and output limits discovered from a provider API.
// Zero values mean the limit could not be determined.
type ModelLimits struct {
	// ContextLength is the model's context window in tokens.
	ContextLength int
	// MaxCompletionTokens is the maximum number of output tokens per turn.
	MaxCompletionTokens int
}
