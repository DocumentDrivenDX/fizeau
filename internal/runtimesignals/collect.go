package runtimesignals

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/easel/fizeau/internal/config"
	"github.com/easel/fizeau/internal/discoverycache"
	claudecache "github.com/easel/fizeau/internal/harnesses/claude"
	codexcache "github.com/easel/fizeau/internal/harnesses/codex"
	geminicache "github.com/easel/fizeau/internal/harnesses/gemini"
	"github.com/easel/fizeau/internal/provider/quotaheaders"
)

const (
	// RuntimeTTL is the default cache TTL for runtime signals (ADR-012 §2).
	RuntimeTTL = 5 * time.Minute
	// RuntimeRefreshDeadline bounds how long a runtime signal refresh may take.
	RuntimeRefreshDeadline = 5 * time.Second

	defaultWindowSize = 100
	aliveCheckTimeout = 3 * time.Second
)

// Store is the per-process in-memory state for runtime signal collection.
// It holds per-provider latency windows and the most recently observed
// rate-limit header signals. All methods are safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	latencies map[string]*LatencyWindow
	headers   *headerStore
}

// NewStore creates an empty Store ready for use.
func NewStore() *Store {
	return &Store{
		latencies: make(map[string]*LatencyWindow),
		headers:   newHeaderStore(),
	}
}

// DefaultStore is the process-singleton Store used by the package-level
// RecordResponse and Collect functions.
var DefaultStore = NewStore()

// RecordResponse records an HTTP response observation for a provider using the
// DefaultStore. providerType controls which header parser is applied
// ("openrouter", "anthropic", or any other value for the OpenAI parser).
func RecordResponse(provider string, h http.Header, latency time.Duration, providerType string) {
	DefaultStore.RecordResponse(provider, h, latency, providerType)
}

// RecordResponse records an HTTP response observation. It updates both the
// latency window and the header signal store for the named provider.
func (s *Store) RecordResponse(provider string, h http.Header, latency time.Duration, providerType string) {
	win := s.latencyWindow(provider)
	win.Record(latency)

	if h != nil {
		now := time.Now()
		var sig quotaheaders.Signal
		switch providerType {
		case "openrouter":
			sig = quotaheaders.ParseOpenRouter(h, now)
		case "anthropic":
			sig = quotaheaders.ParseAnthropic(h, now)
		default:
			sig = quotaheaders.ParseOpenAI(h, now)
		}
		s.headers.record(provider, sig)
	}
}

// Collect assembles a runtime Signal for the named provider using
// DefaultStore.
func Collect(ctx context.Context, providerName string, cfg *config.ProviderConfig) (Signal, error) {
	return DefaultStore.Collect(ctx, providerName, cfg)
}

// Collect assembles a runtime Signal for providerName. The cfg.Type field
// controls which collection path is used:
//
//   - "claude"       → existing internal/harnesses/claude/quota_cache.go
//   - "codex"        → existing internal/harnesses/codex/quota_cache.go
//   - "gemini"       → existing internal/harnesses/gemini/quota_cache.go
//   - "openrouter"   → most recently recorded rate-limit headers (OpenRouter parser)
//   - "openai", "anthropic", and unknown HTTP types → rate-limit headers
//   - local types    → HTTP GET /v1/models alive check (no quota concept)
func (s *Store) Collect(ctx context.Context, providerName string, cfg *config.ProviderConfig) (Signal, error) {
	sig := Signal{
		Provider:   providerName,
		Status:     StatusUnknown,
		RecordedAt: time.Now().UTC(),
	}

	s.mu.RLock()
	win := s.latencies[providerName]
	s.mu.RUnlock()
	if win != nil {
		sig.RecentP50Latency = win.P50()
	}

	providerType := ""
	if cfg != nil {
		providerType = cfg.Type
	}

	switch providerType {
	case "claude":
		collectClaudeSignal(&sig)
	case "codex":
		collectCodexSignal(&sig)
	case "gemini":
		collectGeminiSignal(&sig)
	case "ds4", "llama-server", "omlx", "lmstudio", "lucebox", "vllm", "rapid-mlx", "ollama":
		collectLocalSignal(ctx, cfg, &sig)
	default:
		s.collectHeaderSignal(providerName, &sig)
	}

	return sig, nil
}

// CacheSource returns the discoverycache.Source descriptor for a provider's
// runtime signal. The file path resolves to runtime/<provider>.json.
func CacheSource(providerName string) discoverycache.Source {
	return discoverycache.Source{
		Tier:            "runtime",
		Name:            providerName,
		TTL:             RuntimeTTL,
		RefreshDeadline: RuntimeRefreshDeadline,
	}
}

// Write serializes sig and stores it in M1's runtime cache tier. The cache
// file lands at <cache.Root>/runtime/<sig.Provider>.json.
func Write(cache *discoverycache.Cache, sig Signal) error {
	data, err := json.Marshal(sig)
	if err != nil {
		return err
	}
	src := CacheSource(sig.Provider)
	return cache.Refresh(src, func(_ context.Context) ([]byte, error) {
		return data, nil
	})
}

// ReadCached reads a Signal from M1's runtime cache for providerName.
// Returns (nil, false) when the entry is absent or cannot be decoded.
func ReadCached(cache *discoverycache.Cache, providerName string) (*Signal, bool) {
	src := CacheSource(providerName)
	result, err := cache.Read(src)
	if err != nil || result.Data == nil {
		return nil, false
	}
	var sig Signal
	if err := json.Unmarshal(result.Data, &sig); err != nil {
		return nil, false
	}
	return &sig, true
}

// ---- per-provider-type collectors -------------------------------------------

func collectClaudeSignal(sig *Signal) {
	snap, ok := claudecache.ReadClaudeQuota()
	if !ok || snap == nil {
		return // StatusUnknown
	}
	if claudecache.ClaudeQuotaSnapshotAge(snap, time.Now()) > claudecache.DefaultClaudeQuotaStaleAfter {
		return // StatusUnknown; snapshot is stale
	}
	if snap.WeeklyRemaining <= 0 || snap.FiveHourRemaining <= 0 {
		sig.Status = StatusExhausted
		zero := 0
		sig.QuotaRemaining = &zero
		return
	}
	sig.Status = StatusAvailable
	rem := snap.FiveHourRemaining
	sig.QuotaRemaining = &rem
}

func collectCodexSignal(sig *Signal) {
	snap, ok := codexcache.ReadCodexQuota()
	if !ok || snap == nil {
		return // StatusUnknown
	}
	if codexcache.CodexQuotaSnapshotAge(snap, time.Now()) > codexcache.DefaultCodexQuotaStaleAfter {
		return // StatusUnknown; snapshot is stale
	}
	for _, w := range snap.Windows {
		if w.UsedPercent >= 95 || w.State == "blocked" {
			sig.Status = StatusExhausted
			zero := 0
			sig.QuotaRemaining = &zero
			return
		}
	}
	sig.Status = StatusAvailable
}

func collectGeminiSignal(sig *Signal) {
	snap, ok := geminicache.ReadGeminiQuota()
	if !ok || snap == nil {
		return // StatusUnknown
	}
	if geminicache.GeminiQuotaSnapshotAge(snap, time.Now()) > geminicache.DefaultGeminiQuotaStaleAfter {
		return // StatusUnknown; snapshot is stale
	}
	decision := geminicache.DecideGeminiQuotaRouting(snap, time.Now(), 0)
	if decision.PreferGemini {
		sig.Status = StatusAvailable
	} else {
		sig.Status = StatusExhausted
		zero := 0
		sig.QuotaRemaining = &zero
	}
}

// collectHeaderSignal reads the most recently recorded rate-limit header signal
// for the provider and populates sig accordingly.
func (s *Store) collectHeaderSignal(providerName string, sig *Signal) {
	hSig, ok := s.headers.get(providerName)
	if !ok {
		return // StatusUnknown; no headers observed yet
	}
	exhausted, retryAfter := hSig.IsExhausted(time.Now())
	if exhausted {
		sig.Status = StatusExhausted
		zero := 0
		sig.QuotaRemaining = &zero
		if !retryAfter.IsZero() {
			t := retryAfter
			sig.QuotaResetAt = &t
		}
		return
	}
	sig.Status = StatusAvailable
	switch {
	case hSig.RemainingRequests >= 0:
		rem := int(hSig.RemainingRequests)
		sig.QuotaRemaining = &rem
	case hSig.RemainingTokens >= 0:
		rem := int(hSig.RemainingTokens)
		sig.QuotaRemaining = &rem
	}
	if !hSig.ResetTime.IsZero() {
		t := hSig.ResetTime
		sig.QuotaResetAt = &t
	}
}

// collectLocalSignal performs an HTTP GET /v1/models alive check for local
// providers. Local providers have no quota concept; only StatusAvailable vs
// StatusDegraded is determined.
func collectLocalSignal(ctx context.Context, cfg *config.ProviderConfig, sig *Signal) {
	if cfg == nil || cfg.BaseURL == "" {
		return // StatusUnknown
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, aliveCheckTimeout)
	defer cancel()

	checkURL := cfg.BaseURL + "/v1/models"
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, checkURL, nil) // #nosec G107
	if err != nil {
		return // StatusUnknown; bad URL
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		sig.Status = StatusDegraded
		sig.LastErrorMsg = err.Error()
		now := time.Now().UTC()
		sig.LastErrorAt = &now
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		sig.Status = StatusAvailable
		now := time.Now().UTC()
		sig.LastSuccessAt = &now
	} else {
		sig.Status = StatusDegraded
	}
}

// ---- internal helpers -------------------------------------------------------

func (s *Store) latencyWindow(provider string) *LatencyWindow {
	s.mu.RLock()
	win, ok := s.latencies[provider]
	s.mu.RUnlock()
	if ok {
		return win
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if win, ok = s.latencies[provider]; ok {
		return win
	}
	win = NewLatencyWindow(defaultWindowSize)
	s.latencies[provider] = win
	return win
}
