package main

import "testing"

func TestEffectiveVersionKeepsStampedVersion(t *testing.T) {
	if got := effectiveVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("effectiveVersion(stamped) = %q, want stamped version", got)
	}
}
