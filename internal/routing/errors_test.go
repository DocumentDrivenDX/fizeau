package routing

import (
	"strings"
	"testing"
)

func TestErrNoLiveProviderDistinguishesEmptyVsAllUnhealthy(t *testing.T) {
	allRejected := (&ErrNoLiveProvider{
		StartingPolicy: "smart",
		MinPower:       9,
		MaxPower:       10,
		AllowLocal:     false,
		RejectedCandidates: []CandidateRejection{
			{Harness: "codex", Model: "gpt-5.5", Reason: string(CandidateRejectionQuotaExhausted)},
			{Harness: "claude", Model: "opus-4.7", Reason: string(CandidateRejectionQuotaExhausted)},
			{Harness: "gemini", Model: "gemini-2.5-pro", Reason: string(CandidateRejectionQuotaExhausted)},
		},
	}).Error()
	if !strings.Contains(allRejected, "3 candidate(s)") || !strings.Contains(allRejected, "quota-exhausted") {
		t.Fatalf("all-rejected message=%q, want candidate count and quota bucket", allRejected)
	}

	empty := (&ErrNoLiveProvider{
		StartingPolicy: "smart",
		MinPower:       9,
		MaxPower:       10,
		AllowLocal:     false,
	}).Error()
	if !strings.Contains(empty, "no candidates match policy") || !strings.Contains(empty, "power 9-10") {
		t.Fatalf("empty message=%q, want no-candidate and power range", empty)
	}
}
