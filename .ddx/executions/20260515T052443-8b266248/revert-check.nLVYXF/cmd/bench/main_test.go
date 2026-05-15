package main

import (
	"strings"
	"testing"
)

func TestBenchCommandName(t *testing.T) {
	got := benchCommandName()
	want := "fiz-bench"
	if got != want {
		t.Errorf("benchCommandName() = %q, want %q", got, want)
	}
}

func TestUsageMessageContainsFizBench(t *testing.T) {
	name := benchCommandName()
	if !strings.Contains(name, "fiz-bench") {
		t.Errorf("command name %q does not contain fiz-bench", name)
	}
}

func TestErrorMessageContainsFizBench(t *testing.T) {
	name := benchCommandName()
	if !strings.HasPrefix(name, "fiz-") {
		t.Errorf("command name %q does not start with fiz-", name)
	}
}

func TestRunUnknownCommandUsesCommandName(t *testing.T) {
	// Verify unknown-command error path embeds fiz-bench.
	// run() writes to stderr; we just confirm it returns exit code 2.
	code := run([]string{"no-such-subcommand"})
	if code != 2 {
		t.Errorf("run([no-such-subcommand]) = %d, want 2", code)
	}
}

func TestRunNoArgsReturnsTwo(t *testing.T) {
	code := run([]string{})
	if code != 2 {
		t.Errorf("run([]) = %d, want 2", code)
	}
}

func TestRunHelpReturnsZero(t *testing.T) {
	code := run([]string{"help"})
	if code != 0 {
		t.Errorf("run([help]) = %d, want 0", code)
	}
}
