package fizeau

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	errHarnessModelIncompatible     = errors.New("agent: harness model incompatible")
	errPolicyRequirementUnsatisfied = errors.New("agent: policy requirement unsatisfied")
	errModelConstraintAmbiguous     = errors.New("agent: model constraint ambiguous")
	errModelConstraintNoMatch       = errors.New("agent: model constraint no match")
	errUnknownPolicy                = errors.New("agent: unknown policy")
	errUnsatisfiablePin             = errors.New("agent: unsatisfiable pin")
	errNoLiveProvider               = errors.New("agent: no live provider")
	errUnknownProvider              = errors.New("agent: unknown provider")
	errNoViableProviderForNow       = errors.New("agent: no viable provider for now")
)

// ErrUnknownProvider reports an explicit Provider pin that is not present in
// the service configuration. This is a pre-dispatch pin failure: the caller
// asked for a provider name that the service has no record of, so no route
// can be constructed.
type ErrUnknownProvider struct {
	// Provider is the provider name supplied by the caller.
	Provider string
	// KnownProviders is the set of provider names the service knows about
	// (empty when no ServiceConfig is configured).
	KnownProviders []string
}

func (e ErrUnknownProvider) Error() string {
	if len(e.KnownProviders) == 0 {
		return fmt.Sprintf("unknown provider %q", e.Provider)
	}
	return fmt.Sprintf("unknown provider %q; known providers: %s", e.Provider, strings.Join(e.KnownProviders, ", "))
}

func (e ErrUnknownProvider) Is(target error) bool {
	switch target.(type) {
	case ErrUnknownProvider, *ErrUnknownProvider:
		return true
	default:
		return errors.Is(errUnknownProvider, target)
	}
}

func (e ErrUnknownProvider) Unwrap() error {
	return errUnknownProvider
}

// ErrModelConstraintAmbiguous reports that an explicit Model pin matched more
// than one concrete model after normalization.
type ErrModelConstraintAmbiguous struct {
	// Model is the raw model pin supplied by the caller.
	Model string
	// Candidates are the concrete model IDs that matched the pin.
	Candidates []string
}

func (e ErrModelConstraintAmbiguous) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("ambiguous model %q", e.Model)
	}
	return fmt.Sprintf("ambiguous model %q: candidates: %s", e.Model, strings.Join(e.Candidates, ", "))
}

func (e ErrModelConstraintAmbiguous) Is(target error) bool {
	switch target.(type) {
	case ErrModelConstraintAmbiguous, *ErrModelConstraintAmbiguous:
		return true
	default:
		return errors.Is(errModelConstraintAmbiguous, target)
	}
}

func (e ErrModelConstraintAmbiguous) Unwrap() error {
	return errModelConstraintAmbiguous
}

// ErrModelConstraintNoMatch reports that an explicit Model pin matched no
// discovered or catalog-resolved concrete model IDs.
type ErrModelConstraintNoMatch struct {
	// Model is the raw model pin supplied by the caller.
	Model string
	// Candidates are the nearby candidate IDs considered during resolution.
	Candidates []string
}

func (e ErrModelConstraintNoMatch) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("no matching model for %q", e.Model)
	}
	return fmt.Sprintf("no matching model for %q; nearby candidates: %s", e.Model, strings.Join(e.Candidates, ", "))
}

func (e ErrModelConstraintNoMatch) Is(target error) bool {
	switch target.(type) {
	case ErrModelConstraintNoMatch, *ErrModelConstraintNoMatch:
		return true
	default:
		return errors.Is(errModelConstraintNoMatch, target)
	}
}

func (e ErrModelConstraintNoMatch) Unwrap() error {
	return errModelConstraintNoMatch
}

// DecisionWithCandidates is implemented by routing errors that retain the
// evaluated candidate trace for a failed ResolveRoute call.
type DecisionWithCandidates interface {
	error
	// RouteCandidates returns the evaluated candidates that led to the error.
	RouteCandidates() []RouteCandidate
}

type routeDecisionError struct {
	err        error
	candidates []RouteCandidate
}

func (e *routeDecisionError) Error() string {
	return e.err.Error()
}

func (e *routeDecisionError) Unwrap() error {
	return e.err
}

func (e *routeDecisionError) RouteCandidates() []RouteCandidate {
	return append([]RouteCandidate(nil), e.candidates...)
}

// ErrHarnessModelIncompatible reports an explicit Harness+Model pin that the
// harness allow-list cannot serve.
//
// DDx preflight callers should use errors.As to extract Harness, Model, and
// SupportedModels for worker logs or bead failure records. errors.Is matches a
// zero-value ErrHarnessModelIncompatible, even after callers wrap the error with
// fmt.Errorf and %w.
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

// ErrUnknownPolicy reports an explicit policy name that is not present in the
// model catalog.
type ErrUnknownPolicy struct {
	Policy string
}

func (e ErrUnknownPolicy) Error() string {
	return fmt.Sprintf("unknown routing policy %q", e.Policy)
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
// once.
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

// ErrNoLiveProvider reports that profile-tier escalation walked the entire
// cheap → standard → smart ladder without finding a live provider that
// could serve the request. The message names the prompt size and tool
// requirement so operators know what capability is missing across all
// tiers, rather than the engine's "no viable routing candidate" jargon.
type ErrNoLiveProvider struct {
	PromptTokens   int
	RequiresTools  bool
	StartingPolicy string
}

func (e ErrNoLiveProvider) Error() string {
	return fmt.Sprintf("no live provider supports prompt of %d tokens with tools=%v at policy %s",
		e.PromptTokens, e.RequiresTools, e.StartingPolicy)
}

func (e ErrNoLiveProvider) Is(target error) bool {
	switch target.(type) {
	case ErrNoLiveProvider, *ErrNoLiveProvider:
		return true
	default:
		return errors.Is(errNoLiveProvider, target)
	}
}

func (e ErrNoLiveProvider) Unwrap() error {
	return errNoLiveProvider
}

// NoViableProviderForNow reports that every otherwise-eligible routing
// candidate was disqualified solely because its provider is currently in the
// quota_exhausted state. This is a transient condition: callers (notably DDx)
// should pause work and resume on or after RetryAfter rather than treating
// the request as a permanent failure.
//
// Distinct from ErrNoLiveProvider (entire ladder lacks any live provider) and
// from configuration errors (ErrUnknownProvider, ErrUnknownPolicy,
// ErrHarnessModelIncompatible) which represent operator mistakes that won't
// resolve themselves over time.
type NoViableProviderForNow struct {
	// RetryAfter is the earliest expected provider-recovery time across the
	// exhausted set. Callers should not retry before this instant.
	RetryAfter time.Time
	// ExhaustedProviders is the set of provider names currently in the
	// quota_exhausted state that would otherwise have served the request.
	ExhaustedProviders []string
}

func (e *NoViableProviderForNow) Error() string {
	if len(e.ExhaustedProviders) == 0 {
		return "no viable provider right now: all eligible providers are quota-exhausted"
	}
	return fmt.Sprintf("no viable provider right now: %s quota-exhausted (retry after %s)",
		strings.Join(e.ExhaustedProviders, ", "), e.RetryAfter.Format(time.RFC3339))
}

func (e *NoViableProviderForNow) Is(target error) bool {
	switch target.(type) {
	case *NoViableProviderForNow:
		return true
	default:
		return errors.Is(errNoViableProviderForNow, target)
	}
}

func (e *NoViableProviderForNow) Unwrap() error {
	return errNoViableProviderForNow
}
