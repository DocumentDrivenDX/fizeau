package fizeau

import "github.com/easel/fizeau/internal/reasoning"

func effectiveReasoning(value Reasoning) Reasoning {
	if value != "" {
		normalized, err := reasoning.Normalize(string(value))
		if err == nil {
			return normalized
		}
		return value
	}
	return ""
}

func effectiveReasoningString(value Reasoning) string {
	return string(effectiveReasoning(value))
}
