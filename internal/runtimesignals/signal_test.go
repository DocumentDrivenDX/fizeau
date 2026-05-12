package runtimesignals_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/easel/fizeau/internal/runtimesignals"
)

// TestLatencyWindow_P50 verifies AC4: the sliding-window p50 calculator
// correctly aggregates a stream of latency observations.
func TestLatencyWindow_P50(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		samples []time.Duration
		wantP50 time.Duration
	}{
		{
			name:    "empty returns zero",
			size:    10,
			samples: nil,
			wantP50: 0,
		},
		{
			name:    "single sample",
			size:    10,
			samples: []time.Duration{100 * time.Millisecond},
			wantP50: 100 * time.Millisecond,
		},
		{
			name: "odd count returns median",
			size: 10,
			samples: []time.Duration{
				10 * time.Millisecond,
				50 * time.Millisecond,
				100 * time.Millisecond,
			},
			wantP50: 50 * time.Millisecond,
		},
		{
			name: "even count returns upper-middle",
			size: 10,
			samples: []time.Duration{
				10 * time.Millisecond,
				50 * time.Millisecond,
				75 * time.Millisecond,
				100 * time.Millisecond,
			},
			// sorted[4/2] = sorted[2] = 75ms
			wantP50: 75 * time.Millisecond,
		},
		{
			name: "insertion order does not affect p50",
			size: 10,
			samples: []time.Duration{
				100 * time.Millisecond,
				10 * time.Millisecond,
				50 * time.Millisecond,
			},
			wantP50: 50 * time.Millisecond,
		},
		{
			name: "five uniform samples",
			size: 10,
			samples: []time.Duration{
				20 * time.Millisecond,
				20 * time.Millisecond,
				20 * time.Millisecond,
				20 * time.Millisecond,
				20 * time.Millisecond,
			},
			wantP50: 20 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := runtimesignals.NewLatencyWindow(tt.size)
			for _, s := range tt.samples {
				w.Record(s)
			}
			assert.Equal(t, tt.wantP50, w.P50())
		})
	}
}

// TestLatencyWindow_CircularBuffer verifies that the window discards the oldest
// sample when it wraps around.
func TestLatencyWindow_CircularBuffer(t *testing.T) {
	// size-3 window: record four values so 1ms is overwritten.
	w := runtimesignals.NewLatencyWindow(3)
	w.Record(1 * time.Millisecond)
	w.Record(2 * time.Millisecond)
	w.Record(3 * time.Millisecond)
	w.Record(100 * time.Millisecond) // overwrites 1ms

	// Active samples: {2ms, 3ms, 100ms}. Sorted: [2ms, 3ms, 100ms].
	// P50 = sorted[3/2] = sorted[1] = 3ms.
	assert.Equal(t, 3*time.Millisecond, w.P50())
}

// TestLatencyWindow_ConcurrentSafe is a smoke test that exercises Record and
// P50 from multiple goroutines without triggering the race detector.
func TestLatencyWindow_ConcurrentSafe(t *testing.T) {
	w := runtimesignals.NewLatencyWindow(50)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(ms time.Duration) {
			for j := 0; j < 20; j++ {
				w.Record(ms * time.Millisecond)
			}
			done <- struct{}{}
		}(time.Duration(i + 1))
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	p50 := w.P50()
	assert.GreaterOrEqual(t, int64(p50), int64(time.Millisecond))
}
