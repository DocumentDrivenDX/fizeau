package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestBenchscoreCommandName(t *testing.T) {
	got := benchscoreCommandName()
	want := "fiz-benchscore"
	if got != want {
		t.Errorf("benchscoreCommandName() = %q, want %q", got, want)
	}
}

func TestUsageMessageContainsFizBenchscore(t *testing.T) {
	name := benchscoreCommandName()
	usage := fmt.Sprintf("usage: %s -tasks-jsonl <path>", name)
	if !strings.Contains(usage, "fiz-benchscore") {
		t.Errorf("usage message does not contain fiz-benchscore: %q", usage)
	}
}

func TestErrorMessageContainsFizBenchscore(t *testing.T) {
	name := benchscoreCommandName()
	if !strings.HasPrefix(name, "fiz-") {
		t.Errorf("command name %q does not start with fiz-", name)
	}
}
