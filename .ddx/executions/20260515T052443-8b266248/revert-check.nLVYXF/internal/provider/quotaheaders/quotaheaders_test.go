package quotaheaders

import (
	"net/http"
	"testing"
	"time"
)

var refNow = time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)

func headers(pairs ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}

func TestParseAnthropic_PlentyRemaining(t *testing.T) {
	resetAt := refNow.Add(45 * time.Minute)
	h := headers(
		"anthropic-ratelimit-requests-limit", "1000",
		"anthropic-ratelimit-requests-remaining", "987",
		"anthropic-ratelimit-requests-reset", resetAt.Format(time.RFC3339),
		"anthropic-ratelimit-tokens-limit", "400000",
		"anthropic-ratelimit-tokens-remaining", "350000",
		"anthropic-ratelimit-tokens-reset", resetAt.Format(time.RFC3339),
	)
	sig := ParseAnthropic(h, refNow)
	if !sig.Present {
		t.Fatal("expected Present=true with rate-limit headers populated")
	}
	if sig.RemainingRequests != 987 {
		t.Errorf("RemainingRequests = %d, want 987", sig.RemainingRequests)
	}
	if sig.RemainingTokens != 350000 {
		t.Errorf("RemainingTokens = %d, want 350000", sig.RemainingTokens)
	}
	if !sig.ResetTime.Equal(resetAt) {
		t.Errorf("ResetTime = %v, want %v", sig.ResetTime, resetAt)
	}
	if sig.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0", sig.RetryAfter)
	}
	if exhausted, _ := sig.IsExhausted(refNow); exhausted {
		t.Error("plenty-remaining response should not be exhausted")
	}
}

func TestParseAnthropic_TokensExhausted(t *testing.T) {
	resetAt := refNow.Add(10 * time.Minute)
	h := headers(
		"anthropic-ratelimit-requests-remaining", "100",
		"anthropic-ratelimit-tokens-remaining", "0",
		"anthropic-ratelimit-tokens-reset", resetAt.Format(time.RFC3339),
	)
	sig := ParseAnthropic(h, refNow)
	if sig.RemainingTokens != 0 {
		t.Fatalf("RemainingTokens = %d, want 0", sig.RemainingTokens)
	}
	exhausted, retryAt := sig.IsExhausted(refNow)
	if !exhausted {
		t.Fatal("zero remaining tokens should mark exhausted")
	}
	if !retryAt.Equal(resetAt) {
		t.Errorf("retryAt = %v, want reset %v", retryAt, resetAt)
	}
}

func TestParseAnthropic_RetryAfterSeconds(t *testing.T) {
	h := headers(
		"anthropic-ratelimit-requests-remaining", "10",
		"retry-after", "60",
	)
	sig := ParseAnthropic(h, refNow)
	if sig.RetryAfter != 60*time.Second {
		t.Fatalf("RetryAfter = %v, want 60s", sig.RetryAfter)
	}
	exhausted, retryAt := sig.IsExhausted(refNow)
	if !exhausted {
		t.Fatal("Retry-After should mark exhausted regardless of remaining")
	}
	if !retryAt.Equal(refNow.Add(60 * time.Second)) {
		t.Errorf("retryAt = %v, want now+60s", retryAt)
	}
}

func TestParseAnthropic_PicksMinimumOfPerAxisTokens(t *testing.T) {
	resetEarly := refNow.Add(5 * time.Minute)
	resetLate := refNow.Add(2 * time.Hour)
	h := headers(
		"anthropic-ratelimit-input-tokens-remaining", "9000",
		"anthropic-ratelimit-input-tokens-reset", resetLate.Format(time.RFC3339),
		"anthropic-ratelimit-output-tokens-remaining", "120",
		"anthropic-ratelimit-output-tokens-reset", resetEarly.Format(time.RFC3339),
	)
	sig := ParseAnthropic(h, refNow)
	if sig.RemainingTokens != 120 {
		t.Errorf("RemainingTokens = %d, want min(9000,120)=120", sig.RemainingTokens)
	}
	if !sig.ResetTime.Equal(resetEarly) {
		t.Errorf("ResetTime = %v, want earliest %v", sig.ResetTime, resetEarly)
	}
}

func TestParseAnthropic_NoHeaders(t *testing.T) {
	sig := ParseAnthropic(http.Header{}, refNow)
	if sig.Present {
		t.Fatal("empty headers must yield Present=false")
	}
	if sig.RemainingTokens != -1 || sig.RemainingRequests != -1 {
		t.Errorf("missing headers should map to -1 sentinel, got tokens=%d requests=%d", sig.RemainingTokens, sig.RemainingRequests)
	}
	if exhausted, _ := sig.IsExhausted(refNow); exhausted {
		t.Error("absent signal must not flag exhaustion")
	}
}

func TestParseOpenAI_PlentyRemaining(t *testing.T) {
	h := headers(
		"x-ratelimit-limit-requests", "10000",
		"x-ratelimit-limit-tokens", "1000000",
		"x-ratelimit-remaining-requests", "9876",
		"x-ratelimit-remaining-tokens", "987654",
		"x-ratelimit-reset-requests", "6s",
		"x-ratelimit-reset-tokens", "1m30s",
	)
	sig := ParseOpenAI(h, refNow)
	if !sig.Present {
		t.Fatal("expected Present=true")
	}
	if sig.RemainingRequests != 9876 {
		t.Errorf("RemainingRequests = %d, want 9876", sig.RemainingRequests)
	}
	if sig.RemainingTokens != 987654 {
		t.Errorf("RemainingTokens = %d, want 987654", sig.RemainingTokens)
	}
	wantReset := refNow.Add(6 * time.Second) // earliest of reset-requests/tokens
	if !sig.ResetTime.Equal(wantReset) {
		t.Errorf("ResetTime = %v, want %v", sig.ResetTime, wantReset)
	}
	if exhausted, _ := sig.IsExhausted(refNow); exhausted {
		t.Error("plenty-remaining OpenAI should not be exhausted")
	}
}

func TestParseOpenAI_RequestsZero(t *testing.T) {
	h := headers(
		"x-ratelimit-remaining-requests", "0",
		"x-ratelimit-remaining-tokens", "5000",
		"x-ratelimit-reset-requests", "30s",
	)
	sig := ParseOpenAI(h, refNow)
	exhausted, retryAt := sig.IsExhausted(refNow)
	if !exhausted {
		t.Fatal("remaining-requests=0 should be exhausted")
	}
	want := refNow.Add(30 * time.Second)
	if !retryAt.Equal(want) {
		t.Errorf("retryAt = %v, want %v", retryAt, want)
	}
}

func TestParseOpenAI_RetryAfterHTTPDate(t *testing.T) {
	retryTarget := refNow.Add(5 * time.Minute).Truncate(time.Second)
	h := headers(
		"x-ratelimit-remaining-requests", "100",
		"retry-after", retryTarget.UTC().Format(http.TimeFormat),
	)
	sig := ParseOpenAI(h, refNow)
	if sig.RetryAfter <= 0 {
		t.Fatalf("expected RetryAfter > 0, got %v", sig.RetryAfter)
	}
	exhausted, _ := sig.IsExhausted(refNow)
	if !exhausted {
		t.Error("Retry-After should drive exhaustion")
	}
}

func TestParseOpenRouter_PlentyRemaining(t *testing.T) {
	resetAt := refNow.Add(20 * time.Minute)
	h := headers(
		"x-ratelimit-limit", "1000",
		"x-ratelimit-remaining", "812",
		"x-ratelimit-reset", strconvFormatMs(resetAt),
	)
	sig := ParseOpenRouter(h, refNow)
	if !sig.Present {
		t.Fatal("expected Present=true")
	}
	if sig.RemainingRequests != 812 {
		t.Errorf("RemainingRequests = %d, want 812", sig.RemainingRequests)
	}
	if sig.RemainingTokens != -1 {
		t.Errorf("RemainingTokens = %d, want -1 (OpenRouter does not report tokens)", sig.RemainingTokens)
	}
	if !sig.ResetTime.Equal(resetAt) {
		t.Errorf("ResetTime = %v, want %v", sig.ResetTime, resetAt)
	}
	if exhausted, _ := sig.IsExhausted(refNow); exhausted {
		t.Error("plenty-remaining OpenRouter should not be exhausted")
	}
}

func TestParseOpenRouter_DailyExhausted(t *testing.T) {
	resetAt := refNow.Add(6 * time.Hour) // OpenRouter daily window
	h := headers(
		"x-ratelimit-limit", "200",
		"x-ratelimit-remaining", "0",
		"x-ratelimit-reset", strconvFormatMs(resetAt),
	)
	sig := ParseOpenRouter(h, refNow)
	exhausted, retryAt := sig.IsExhausted(refNow)
	if !exhausted {
		t.Fatal("remaining=0 should mark exhausted")
	}
	if !retryAt.Equal(resetAt.UTC()) {
		t.Errorf("retryAt = %v, want %v", retryAt, resetAt.UTC())
	}
}

func TestParseOpenRouter_RetryAfterTakesPrecedence(t *testing.T) {
	resetAt := refNow.Add(6 * time.Hour)
	h := headers(
		"x-ratelimit-remaining", "5",
		"x-ratelimit-reset", strconvFormatMs(resetAt),
		"retry-after", "120",
	)
	sig := ParseOpenRouter(h, refNow)
	exhausted, retryAt := sig.IsExhausted(refNow)
	if !exhausted {
		t.Fatal("Retry-After should mark exhausted")
	}
	want := refNow.Add(2 * time.Minute)
	if !retryAt.Equal(want) {
		t.Errorf("retryAt = %v, want %v (Retry-After overrides reset)", retryAt, want)
	}
}

func TestNilHeaders(t *testing.T) {
	for _, parse := range []func(http.Header, time.Time) Signal{ParseAnthropic, ParseOpenAI, ParseOpenRouter} {
		sig := parse(nil, refNow)
		if sig.Present {
			t.Errorf("nil headers should yield Present=false")
		}
		if exhausted, _ := sig.IsExhausted(refNow); exhausted {
			t.Errorf("nil headers should never flag exhaustion")
		}
	}
}

// strconvFormatMs renders a wall-clock time as the unix-ms string that
// OpenRouter uses for x-ratelimit-reset.
func strconvFormatMs(t time.Time) string {
	ms := t.UnixMilli()
	return formatInt(ms)
}

func formatInt(v int64) string {
	// Avoid pulling strconv into the test boilerplate header; this trivial
	// itoa keeps the helper local and obvious.
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
