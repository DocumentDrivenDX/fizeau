package openaicompat

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	agent "github.com/easel/fizeau/internal/core"
)

// TestOpenAIClient_DialTimeoutDefault5s asserts that the openaicompat.NewClient
// default Transport has an explicit DialContext with timeout ≤5s. Without this
// bound the kernel waits the full SYN-retransmit window (~30s on Linux) before
// surfacing a dial-class failure to the retry-loop short-circuit.
func TestOpenAIClient_DialTimeoutDefault5s(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if c.connectTimeout <= 0 {
		t.Fatalf("connect timeout must be positive, got %v", c.connectTimeout)
	}
	if c.connectTimeout > 5*time.Second {
		t.Errorf("default connect timeout = %v, want ≤5s", c.connectTimeout)
	}
	if c.connectTimeout != DefaultConnectTimeout {
		t.Errorf("default connect timeout = %v, want %v", c.connectTimeout, DefaultConnectTimeout)
	}
}

// TestOpenAIClient_DialTimeoutConfigurable asserts ConnectTimeout in Config
// overrides the default. The override path is what DDx / benchmarks would use
// on high-latency networks where 5s is too aggressive.
func TestOpenAIClient_DialTimeoutConfigurable(t *testing.T) {
	cases := []struct {
		name string
		cfg  time.Duration
		want time.Duration
	}{
		{"shorter_than_default", 2 * time.Second, 2 * time.Second},
		{"longer_than_default", 15 * time.Second, 15 * time.Second},
		{"zero_falls_back_to_default", 0, DefaultConnectTimeout},
		{"negative_falls_back_to_default", -1 * time.Second, DefaultConnectTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(Config{BaseURL: "http://127.0.0.1:1", ConnectTimeout: tc.cfg})
			if c.connectTimeout != tc.want {
				t.Errorf("connect timeout = %v, want %v", c.connectTimeout, tc.want)
			}
		})
	}
}

// TestProviderDeadLocalHost_FailsUnder5s aims at an unroutable RFC 5737
// address and asserts the provider call returns within 5.5s with a dial error.
// No process listening means the kernel-level connect timeout exercises the
// bounded dial; without our DialContext, this call would block ~30s on Linux.
func TestProviderDeadLocalHost_FailsUnder5s(t *testing.T) {
	if testing.Short() {
		t.Skip("network-touching test skipped in -short mode")
	}
	// RFC 5737 TEST-NET-1: documented-as-unroutable. Port 1 has no listener.
	c := NewClient(Config{BaseURL: "http://192.0.2.1:1"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	_, err := c.Chat(ctx, "model", []agent.Message{{Role: agent.RoleUser, Content: "hi"}}, nil, RequestOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
	if elapsed > 5500*time.Millisecond {
		t.Errorf("dial took %v, want ≤5.5s (bounded by ConnectTimeout)", elapsed)
	}
	// The wrapped error should report a dial-class failure (either *net.OpError
	// with Op=="dial" or a context.DeadlineExceeded from the Dialer.Timeout path).
	var opErr *net.OpError
	hasOp := errors.As(err, &opErr) && opErr.Op == "dial"
	hasTimeoutText := strings.Contains(strings.ToLower(err.Error()), "timeout") ||
		strings.Contains(strings.ToLower(err.Error()), "i/o timeout") ||
		strings.Contains(strings.ToLower(err.Error()), "deadline exceeded")
	if !hasOp && !hasTimeoutText {
		t.Errorf("expected dial-class error, got: %v", err)
	}
}
