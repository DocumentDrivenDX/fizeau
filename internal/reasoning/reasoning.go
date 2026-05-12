package reasoning

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reasoning is the canonical scalar for model-side reasoning controls.
type Reasoning string

const (
	ReasoningAuto    Reasoning = "auto"
	ReasoningOff     Reasoning = "off"
	ReasoningLow     Reasoning = "low"
	ReasoningMedium  Reasoning = "medium"
	ReasoningHigh    Reasoning = "high"
	ReasoningMinimal Reasoning = "minimal"
	ReasoningXHigh   Reasoning = "xhigh"
	ReasoningMax     Reasoning = "max"
)

// PortableBudgets are fallback named reasoning token budgets.
var PortableBudgets = map[Reasoning]int{
	ReasoningOff:    0,
	ReasoningLow:    2048,
	ReasoningMedium: 8192,
	ReasoningHigh:   32768,
}

// ReasoningTokens returns a numeric reasoning-token request.
func ReasoningTokens(n int) Reasoning {
	return Reasoning(strconv.Itoa(n))
}

type Kind string

const (
	KindUnset  Kind = "unset"
	KindAuto   Kind = "auto"
	KindOff    Kind = "off"
	KindNamed  Kind = "named"
	KindTokens Kind = "tokens"
)

type Policy struct {
	Kind   Kind
	Value  Reasoning
	Tokens int
}

type ResolutionSource string

const (
	ResolutionSourceCaller  ResolutionSource = "caller"
	ResolutionSourceSnapped ResolutionSource = "snapped"
	ResolutionSourceDefault ResolutionSource = "default"
)

type SupportedResolution struct {
	Policy    Policy
	Source    ResolutionSource
	Warning   string
	Reason    string
	Supported []Reasoning
}

func (p Policy) IsSet() bool {
	return p.Kind != KindUnset
}

func (p Policy) IsExplicitOff() bool {
	return p.Kind == KindOff || (p.Kind == KindTokens && p.Tokens == 0)
}

func Normalize(value string) (Reasoning, error) {
	p, err := ParseString(value)
	if err != nil {
		return "", err
	}
	return p.Value, nil
}

func ParseString(value string) (Policy, error) {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" {
		return Policy{Kind: KindUnset}, nil
	}
	switch s {
	case "auto":
		return Policy{Kind: KindAuto, Value: ReasoningAuto}, nil
	case "off", "none", "false":
		return Policy{Kind: KindOff, Value: ReasoningOff}, nil
	case "low":
		return Policy{Kind: KindNamed, Value: ReasoningLow}, nil
	case "medium":
		return Policy{Kind: KindNamed, Value: ReasoningMedium}, nil
	case "high":
		return Policy{Kind: KindNamed, Value: ReasoningHigh}, nil
	case "minimal":
		return Policy{Kind: KindNamed, Value: ReasoningMinimal}, nil
	case "x-high", "x_high", "xhigh":
		return Policy{Kind: KindNamed, Value: ReasoningXHigh}, nil
	case "max":
		return Policy{Kind: KindNamed, Value: ReasoningMax}, nil
	}
	tokens, err := strconv.Atoi(s)
	if err == nil {
		if tokens < 0 {
			return Policy{}, fmt.Errorf("reasoning: negative token budget %d is invalid", tokens)
		}
		if tokens == 0 {
			return Policy{Kind: KindOff, Value: ReasoningOff, Tokens: 0}, nil
		}
		return Policy{Kind: KindTokens, Value: ReasoningTokens(tokens), Tokens: tokens}, nil
	}
	return Policy{}, fmt.Errorf("reasoning: unsupported value %q", value)
}

func Parse(value any) (Policy, error) {
	switch v := value.(type) {
	case nil:
		return Policy{Kind: KindUnset}, nil
	case Reasoning:
		return ParseString(string(v))
	case string:
		return ParseString(v)
	case int:
		return ParseString(strconv.Itoa(v))
	case int64:
		return ParseString(strconv.FormatInt(v, 10))
	case float64:
		if v != float64(int(v)) {
			return Policy{}, fmt.Errorf("reasoning: numeric value %v must be an integer", v)
		}
		return ParseString(strconv.Itoa(int(v)))
	case json.Number:
		i, err := strconv.Atoi(v.String())
		if err != nil {
			return Policy{}, fmt.Errorf("reasoning: numeric value %q must be an integer", v.String())
		}
		return ParseString(strconv.Itoa(i))
	default:
		return Policy{}, fmt.Errorf("reasoning: unsupported scalar type %T", value)
	}
}

// BudgetForNamed returns the PortableBudgets token budget for a named reasoning
// tier. Returns 0 for tiers without a portable budget (off, minimal, xhigh, max).
func BudgetForNamed(r Reasoning) int {
	return PortableBudgets[r]
}

// NearestTierForTokens snaps a token count to the nearest PortableBudgets tier
// using log2-scale distance; ties round up to the higher tier.
// Boundaries (geometric midpoints): <4096 → low, [4096,16384) → medium, ≥16384 → high.
func NearestTierForTokens(n int) Reasoning {
	if n < 4096 {
		return ReasoningLow
	}
	if n < 16384 {
		return ReasoningMedium
	}
	return ReasoningHigh
}

// ResolveAgainstSupportedLevels validates a caller policy against discovered
// harness reasoning levels. Empty or unparsable support lists deliberately keep
// the existing behavior: the caller policy is returned unchanged.
func ResolveAgainstSupportedLevels(policy Policy, supportedLevels []string) (SupportedResolution, error) {
	supported := normalizeSupportedLevels(supportedLevels)
	source := ResolutionSourceCaller
	if policy.Kind == KindUnset || policy.Kind == KindAuto {
		source = ResolutionSourceDefault
	}
	out := SupportedResolution{
		Policy:    policy,
		Source:    source,
		Supported: supported,
	}
	if len(supported) == 0 {
		return out, nil
	}
	switch policy.Kind {
	case KindUnset, KindAuto, KindOff:
		return out, nil
	case KindTokens:
		tier := NearestTierForTokens(policy.Tokens)
		if containsReasoning(supported, tier) {
			out.Policy = Policy{Kind: KindNamed, Value: tier}
			out.Source = ResolutionSourceSnapped
			out.Reason = "tokens_converted_to_effort"
			return out, nil
		}
		snapped, ok := nearestSupportedTier(tier, supported)
		if !ok {
			return out, nil
		}
		out.Policy = Policy{Kind: KindNamed, Value: snapped}
		out.Source = ResolutionSourceSnapped
		out.Reason = "tokens_converted_and_snapped_to_supported_effort"
		out.Warning = fmt.Sprintf("reasoning effort %q from %d tokens is not supported; snapped to %q", tier, policy.Tokens, snapped)
		return out, nil
	case KindNamed:
		if containsReasoning(supported, policy.Value) {
			return out, nil
		}
		snapped, ok := nearestSupportedTier(policy.Value, supported)
		if !ok {
			return out, nil
		}
		out.Policy = Policy{Kind: KindNamed, Value: snapped}
		out.Source = ResolutionSourceSnapped
		out.Reason = "unsupported_effort_snapped_to_nearest_supported"
		out.Warning = fmt.Sprintf("reasoning effort %q is not supported; snapped to %q", policy.Value, snapped)
		return out, nil
	default:
		return SupportedResolution{}, fmt.Errorf("reasoning: unsupported policy kind %q", policy.Kind)
	}
}

func normalizeSupportedLevels(levels []string) []Reasoning {
	out := make([]Reasoning, 0, len(levels))
	for _, level := range levels {
		policy, err := ParseString(level)
		if err != nil {
			continue
		}
		switch policy.Kind {
		case KindOff, KindNamed:
			if !containsReasoning(out, policy.Value) {
				out = append(out, policy.Value)
			}
		}
	}
	return out
}

func containsReasoning(values []Reasoning, value Reasoning) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func nearestSupportedTier(requested Reasoning, supported []Reasoning) (Reasoning, bool) {
	requestBudget, ok := distanceBudget(requested)
	if !ok {
		return "", false
	}
	var best Reasoning
	var bestBudget int
	bestDistance := math.Inf(1)
	for _, candidate := range supported {
		candidateBudget, ok := distanceBudget(candidate)
		if !ok {
			continue
		}
		distance := math.Abs(math.Log2(float64(candidateBudget)) - math.Log2(float64(requestBudget)))
		if best == "" || distance < bestDistance || (distance == bestDistance && candidateBudget > bestBudget) {
			best = candidate
			bestBudget = candidateBudget
			bestDistance = distance
		}
	}
	return best, best != ""
}

func distanceBudget(value Reasoning) (int, bool) {
	switch value {
	case ReasoningMinimal:
		return PortableBudgets[ReasoningLow], true
	case ReasoningXHigh, ReasoningMax:
		return PortableBudgets[ReasoningHigh], true
	}
	budget, ok := PortableBudgets[value]
	if !ok || budget <= 0 {
		return 0, false
	}
	return budget, true
}

func BudgetFor(policy Policy, budgets map[Reasoning]int, maxTokens int) (int, error) {
	switch policy.Kind {
	case KindUnset, KindAuto:
		return 0, nil
	case KindOff:
		return 0, nil
	case KindTokens:
		if maxTokens > 0 && policy.Tokens > maxTokens {
			return 0, fmt.Errorf("reasoning: token budget %d exceeds maximum %d", policy.Tokens, maxTokens)
		}
		return policy.Tokens, nil
	case KindNamed:
		if policy.Value == ReasoningMax {
			if maxTokens <= 0 {
				return 0, fmt.Errorf("reasoning: max requires a known model/provider maximum")
			}
			return maxTokens, nil
		}
		if budgets != nil {
			if budget, ok := budgets[policy.Value]; ok {
				return budget, nil
			}
		}
		if budget, ok := PortableBudgets[policy.Value]; ok {
			if maxTokens > 0 && budget > maxTokens {
				return 0, fmt.Errorf("reasoning: %s budget %d exceeds maximum %d", policy.Value, budget, maxTokens)
			}
			return budget, nil
		}
		return 0, fmt.Errorf("reasoning: unsupported named value %q for numeric budget", policy.Value)
	default:
		return 0, fmt.Errorf("reasoning: unsupported policy kind %q", policy.Kind)
	}
}

func (r *Reasoning) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		normalized, err := Normalize(s)
		if err != nil {
			return err
		}
		*r = normalized
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		policy, err := Parse(n)
		if err != nil {
			return err
		}
		*r = policy.Value
		return nil
	}
	return fmt.Errorf("reasoning: JSON value must be a string or integer")
}

func (r *Reasoning) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		policy, err := ParseString(value.Value)
		if err != nil {
			return err
		}
		*r = policy.Value
		return nil
	default:
		return fmt.Errorf("reasoning: YAML value must be a scalar")
	}
}
