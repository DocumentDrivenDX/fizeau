// Package quotaheaders parses provider-supplied rate-limit / quota response
// headers into a structured Signal. The Signal feeds the per-provider quota
// state machine introduced in fizeau-92b4b823 so dispatch can route around
// providers whose subscription/daily cap is hit (or imminently will be).
//
// This package is intentionally limited to the subscription/daily exhaustion
// case. Per-second / per-minute throttling (a 429 with a short Retry-After)
// stays in the per-request feedback path.
package quotaheaders

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Signal is the structured shape returned by every per-provider parser.
// All fields are zero values when the corresponding header is missing.
//
// The router treats a Signal as actionable only when Present is true; that
// guards against treating a response with no rate-limit headers (e.g. a
// 200 from a self-hosted endpoint) as "remaining tokens = 0".
type Signal struct {
	// Present is true when the response carried at least one recognized
	// rate-limit header. Consumers must check Present before reading the
	// numeric fields.
	Present bool

	// RemainingTokens is the remaining token budget in the current window.
	// -1 means the provider did not report a token budget on this response.
	RemainingTokens int64
	// RemainingRequests is the remaining request budget in the current
	// window. -1 means the provider did not report a request budget.
	RemainingRequests int64

	// ResetTime is the absolute wall-clock instant when the smaller of the
	// remaining-tokens / remaining-requests window resets. Zero when no
	// reset header was supplied.
	ResetTime time.Time

	// RetryAfter is the duration carried by an explicit Retry-After header.
	// Zero when absent. RetryAfter > 0 always indicates the provider has
	// already returned a 429-class response and wants the caller to wait.
	RetryAfter time.Duration
}

// retryAfterTime returns the wall-clock instant the provider wants the caller
// to retry after, or zero when neither RetryAfter nor ResetTime are set.
// RetryAfter takes precedence because it is always provider-authoritative;
// ResetTime is the fall-back when only the reset window is exposed.
func (s Signal) retryAfterTime(now time.Time) time.Time {
	if s.RetryAfter > 0 {
		return now.Add(s.RetryAfter)
	}
	return s.ResetTime
}

// IsExhausted applies the conservative "remaining tokens/requests would not
// last to the next reset" heuristic from the bead. The current rule:
//
//   - explicit Retry-After > 0 (provider already returned 429) — exhausted
//   - RemainingRequests reported and == 0 — exhausted
//   - RemainingTokens reported and == 0 — exhausted
//
// Returns the wall-clock retry-after time (zero when nothing actionable was
// signaled). The "will-exhaust-before-reset" predicate is intentionally
// conservative: only zero-budget triggers exhaustion. That keeps false
// positives out of the dispatch path while still catching the cases where
// the provider has authoritatively said "no more headroom in this window."
func (s Signal) IsExhausted(now time.Time) (bool, time.Time) {
	if !s.Present {
		return false, time.Time{}
	}
	if s.RetryAfter > 0 {
		return true, s.retryAfterTime(now)
	}
	if s.RemainingRequests == 0 || s.RemainingTokens == 0 {
		// "0" only counts when the header was actually present. The
		// parsers use -1 for "not reported" so a missing header does
		// not look like exhaustion.
		return true, s.retryAfterTime(now)
	}
	return false, time.Time{}
}

// ParseAnthropic decodes Anthropic Messages-API rate-limit headers.
//
// Recognized canonical headers (current Anthropic docs):
//
//	anthropic-ratelimit-requests-{limit,remaining,reset}
//	anthropic-ratelimit-tokens-{limit,remaining,reset}
//	anthropic-ratelimit-input-tokens-{limit,remaining,reset}
//	anthropic-ratelimit-output-tokens-{limit,remaining,reset}
//	retry-after
//
// The bead also references the legacy "X-RateLimit-Remaining-*" family;
// those names are accepted for forward-compatibility but the canonical
// "anthropic-ratelimit-*" names are preferred when both are present.
//
// Reset values are RFC3339 timestamps (Anthropic) and are returned as the
// minimum of the per-axis reset times so the consumer wakes at the earliest
// recovery boundary.
func ParseAnthropic(h http.Header, now time.Time) Signal {
	if h == nil {
		return Signal{RemainingTokens: -1, RemainingRequests: -1}
	}
	sig := Signal{RemainingTokens: -1, RemainingRequests: -1}

	requestsRem, requestsRemOK := readInt(h, "anthropic-ratelimit-requests-remaining", "x-ratelimit-remaining-requests")
	if requestsRemOK {
		sig.Present = true
		sig.RemainingRequests = requestsRem
	}

	// Anthropic exposes total-tokens, plus per-axis input/output budgets.
	// Track the smallest reported "remaining tokens" since the smallest
	// budget is the binding one for the next request.
	tokenAxes := []string{
		"anthropic-ratelimit-tokens-remaining",
		"anthropic-ratelimit-input-tokens-remaining",
		"anthropic-ratelimit-output-tokens-remaining",
		"x-ratelimit-remaining-tokens",
	}
	for _, name := range tokenAxes {
		v, ok := readInt(h, name)
		if !ok {
			continue
		}
		sig.Present = true
		if sig.RemainingTokens < 0 || v < sig.RemainingTokens {
			sig.RemainingTokens = v
		}
	}

	resetAxes := []string{
		"anthropic-ratelimit-requests-reset",
		"anthropic-ratelimit-tokens-reset",
		"anthropic-ratelimit-input-tokens-reset",
		"anthropic-ratelimit-output-tokens-reset",
	}
	for _, name := range resetAxes {
		raw := strings.TrimSpace(h.Get(name))
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			continue
		}
		sig.Present = true
		if sig.ResetTime.IsZero() || t.Before(sig.ResetTime) {
			sig.ResetTime = t
		}
	}

	if d, ok := readRetryAfter(h, now); ok {
		sig.Present = true
		sig.RetryAfter = d
	}

	return sig
}

// ParseOpenAI decodes OpenAI Chat Completions rate-limit headers.
//
// Recognized headers (https://platform.openai.com/docs/guides/rate-limits):
//
//	x-ratelimit-limit-requests
//	x-ratelimit-limit-tokens
//	x-ratelimit-remaining-requests
//	x-ratelimit-remaining-tokens
//	x-ratelimit-reset-requests   (duration: "1s", "100ms", "2m30s")
//	x-ratelimit-reset-tokens     (duration)
//	retry-after
//
// Reset values are durations; this parser converts them to absolute times
// using `now`.
func ParseOpenAI(h http.Header, now time.Time) Signal {
	if h == nil {
		return Signal{RemainingTokens: -1, RemainingRequests: -1}
	}
	sig := Signal{RemainingTokens: -1, RemainingRequests: -1}

	if v, ok := readInt(h, "x-ratelimit-remaining-requests"); ok {
		sig.Present = true
		sig.RemainingRequests = v
	}
	if v, ok := readInt(h, "x-ratelimit-remaining-tokens"); ok {
		sig.Present = true
		sig.RemainingTokens = v
	}

	for _, name := range []string{"x-ratelimit-reset-requests", "x-ratelimit-reset-tokens"} {
		raw := strings.TrimSpace(h.Get(name))
		if raw == "" {
			continue
		}
		d, err := parseOpenAIDuration(raw)
		if err != nil {
			continue
		}
		sig.Present = true
		t := now.Add(d)
		if sig.ResetTime.IsZero() || t.Before(sig.ResetTime) {
			sig.ResetTime = t
		}
	}

	if d, ok := readRetryAfter(h, now); ok {
		sig.Present = true
		sig.RetryAfter = d
	}

	return sig
}

// ParseOpenRouter decodes OpenRouter rate-limit headers.
//
// Recognized headers (https://openrouter.ai/docs/limits):
//
//	x-ratelimit-limit
//	x-ratelimit-remaining
//	x-ratelimit-reset    (Unix ms timestamp)
//	retry-after
//
// OpenRouter expresses one combined budget rather than separate token /
// request axes; the parser maps it onto RemainingRequests and leaves
// RemainingTokens at -1 ("not reported").
func ParseOpenRouter(h http.Header, now time.Time) Signal {
	if h == nil {
		return Signal{RemainingTokens: -1, RemainingRequests: -1}
	}
	sig := Signal{RemainingTokens: -1, RemainingRequests: -1}

	if v, ok := readInt(h, "x-ratelimit-remaining"); ok {
		sig.Present = true
		sig.RemainingRequests = v
	}

	if raw := strings.TrimSpace(h.Get("x-ratelimit-reset")); raw != "" {
		if ms, err := strconv.ParseInt(raw, 10, 64); err == nil && ms > 0 {
			sig.Present = true
			sig.ResetTime = time.UnixMilli(ms).UTC()
		}
	}

	if d, ok := readRetryAfter(h, now); ok {
		sig.Present = true
		sig.RetryAfter = d
	}

	return sig
}

// readInt looks up the first non-empty header value in `names` and parses it
// as a 64-bit integer. The "ok" return distinguishes "header absent" from
// "header present and zero" — important for IsExhausted, which treats a
// reported zero as exhaustion but a missing header as no signal.
func readInt(h http.Header, names ...string) (int64, bool) {
	for _, name := range names {
		raw := strings.TrimSpace(h.Get(name))
		if raw == "" {
			continue
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		return v, true
	}
	return 0, false
}

// readRetryAfter parses an HTTP Retry-After header into a positive duration.
// Per RFC 7231 §7.1.3 the value is either delta-seconds or an HTTP-date.
func readRetryAfter(h http.Header, now time.Time) (time.Duration, bool) {
	raw := strings.TrimSpace(h.Get("retry-after"))
	if raw == "" {
		return 0, false
	}
	if secs, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if secs <= 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := t.Sub(now)
		if d <= 0 {
			return 0, false
		}
		return d, true
	}
	return 0, false
}

// parseOpenAIDuration parses the OpenAI compound-duration form ("2m30s",
// "100ms", "1s") into a time.Duration. time.ParseDuration handles every
// shape OpenAI emits today; this wrapper exists so tests can reach the same
// helper directly when verifying edge cases.
func parseOpenAIDuration(raw string) (time.Duration, error) {
	return time.ParseDuration(raw)
}
