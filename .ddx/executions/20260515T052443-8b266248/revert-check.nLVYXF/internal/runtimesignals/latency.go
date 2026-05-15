package runtimesignals

import (
	"sort"
	"sync"
	"time"
)

// LatencyWindow tracks recent request latencies in a fixed-size circular
// buffer. When the buffer is full, the oldest sample is overwritten. P50 is
// computed over all samples currently in the window. The window is safe for
// concurrent use.
type LatencyWindow struct {
	mu      sync.Mutex
	samples []time.Duration
	head    int
	count   int
}

// NewLatencyWindow creates a LatencyWindow that retains the last n samples.
func NewLatencyWindow(n int) *LatencyWindow {
	if n <= 0 {
		n = 100
	}
	return &LatencyWindow{samples: make([]time.Duration, n)}
}

// Record adds one latency observation. When the buffer is full, the oldest
// sample is silently discarded.
func (w *LatencyWindow) Record(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.samples[w.head] = d
	w.head = (w.head + 1) % len(w.samples)
	if w.count < len(w.samples) {
		w.count++
	}
}

// P50 returns the 50th-percentile latency across the current window.
// Returns 0 when no samples have been recorded. For an even number of
// samples, the upper-middle value is returned (index count/2 after sorting).
func (w *LatencyWindow) P50() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.count == 0 {
		return 0
	}
	// samples[:count] holds all valid entries regardless of wrap position:
	// when count < cap, only the first count slots are set; when count == cap
	// the full slice is occupied (circular order, but sort neutralises that).
	sorted := make([]time.Duration, w.count)
	copy(sorted, w.samples[:w.count])
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[w.count/2]
}
