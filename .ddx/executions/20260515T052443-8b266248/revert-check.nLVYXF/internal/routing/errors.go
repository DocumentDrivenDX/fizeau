package routing

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	errHarnessModelIncompatible     = errors.New("routing: harness model incompatible")
	errPolicyRequirementUnsatisfied = errors.New("routing: policy requirement unsatisfied")
	errUnknownPolicy                = errors.New("routing: unknown policy")
	errUnsatisfiablePin             = errors.New("routing: unsatisfiable pin")
	errAllProvidersQuotaExhausted   = errors.New("routing: all providers quota exhausted")
)

// ErrHarnessModelIncompatible reports an explicit Harness+Model pin that the
// harness allow-list cannot serve.
type ErrHarnessModelIncompatible struct {
	// Harness is the canonical harness name supplied by the caller.
	Harness string
	// Model is the exact concrete model pin supplied by the caller.
	Model string
	// SupportedModels is the harness allow-list that rejected Model.
	SupportedModels []string
}

func (e ErrHarnessModelIncompatible) Error() string {
	return fmt.Sprintf("model %q is not supported by harness %q; supported models: %s", e.Model, e.Harness, strings.Join(e.SupportedModels, ", "))
}

func (e ErrHarnessModelIncompatible) Is(target error) bool {
	switch target.(type) {
	case ErrHarnessModelIncompatible, *ErrHarnessModelIncompatible:
		return true
	default:
		return errors.Is(errHarnessModelIncompatible, target)
	}
}

func (e ErrHarnessModelIncompatible) Unwrap() error {
	return errHarnessModelIncompatible
}

// ErrPolicyRequirementUnsatisfied reports a hard policy requirement that no
// candidate, or an explicit caller pin, can satisfy.
type ErrPolicyRequirementUnsatisfied struct {
	Policy       string
	Requirement  string
	AttemptedPin string
	Rejected     int
}

func (e ErrPolicyRequirementUnsatisfied) Error() string {
	if e.AttemptedPin != "" {
		return fmt.Sprintf("policy %q requires %s but conflicts with %s", e.Policy, e.Requirement, e.AttemptedPin)
	}
	return fmt.Sprintf("policy %q has no candidate satisfying %s: %d candidates rejected", e.Policy, e.Requirement, e.Rejected)
}

func (e ErrPolicyRequirementUnsatisfied) Is(target error) bool {
	switch target.(type) {
	case ErrPolicyRequirementUnsatisfied, *ErrPolicyRequirementUnsatisfied:
		return true
	default:
		return errors.Is(errPolicyRequirementUnsatisfied, target)
	}
}

func (e ErrPolicyRequirementUnsatisfied) Unwrap() error {
	return errPolicyRequirementUnsatisfied
}

// ErrUnknownPolicy reports a policy name outside the routing engine's
// canonical v0.11 policy vocabulary.
type ErrUnknownPolicy struct {
	Policy string
}

func (e ErrUnknownPolicy) Error() string {
	return fmt.Sprintf("unknown policy %q", e.Policy)
}

func (e ErrUnknownPolicy) Is(target error) bool {
	switch target.(type) {
	case ErrUnknownPolicy, *ErrUnknownPolicy:
		return true
	default:
		return errors.Is(errUnknownPolicy, target)
	}
}

func (e ErrUnknownPolicy) Unwrap() error {
	return errUnknownPolicy
}

// ErrUnsatisfiablePin reports explicit caller pins that cannot all be true at
// once, such as a harness/model pair where the harness cannot serve the model.
type ErrUnsatisfiablePin struct {
	Pin    string
	Reason string
}

func (e ErrUnsatisfiablePin) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("unsatisfiable pin %s", e.Pin)
	}
	return fmt.Sprintf("unsatisfiable pin %s: %s", e.Pin, e.Reason)
}

func (e ErrUnsatisfiablePin) Is(target error) bool {
	switch target.(type) {
	case ErrUnsatisfiablePin, *ErrUnsatisfiablePin:
		return true
	default:
		return errors.Is(errUnsatisfiablePin, target)
	}
}

func (e ErrUnsatisfiablePin) Unwrap() error {
	return errUnsatisfiablePin
}

// ErrAllProvidersQuotaExhausted reports that every routing candidate that
// would have been eligible for the request was filtered out solely because
// its provider is currently in quota_exhausted state. RetryAfter is the
// earliest expected provider-recovery time across the exhausted set.
type ErrAllProvidersQuotaExhausted struct {
	RetryAfter         time.Time
	ExhaustedProviders []string
}

func (e ErrAllProvidersQuotaExhausted) Error() string {
	if len(e.ExhaustedProviders) == 0 {
		return "all eligible providers are quota-exhausted"
	}
	return fmt.Sprintf("all eligible providers are quota-exhausted: %s (retry after %s)",
		strings.Join(e.ExhaustedProviders, ", "), e.RetryAfter.Format(time.RFC3339))
}

func (e ErrAllProvidersQuotaExhausted) Is(target error) bool {
	switch target.(type) {
	case ErrAllProvidersQuotaExhausted, *ErrAllProvidersQuotaExhausted:
		return true
	default:
		return errors.Is(errAllProvidersQuotaExhausted, target)
	}
}

func (e ErrAllProvidersQuotaExhausted) Unwrap() error {
	return errAllProvidersQuotaExhausted
}
