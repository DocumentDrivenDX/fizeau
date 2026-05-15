package serviceimpl

import (
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

func TestRuntimeDepsClockDefaultsToUTCSystemClock(t *testing.T) {
	clk := RuntimeDeps{}.Clock()
	if got := clk(); got.Location() != time.UTC {
		t.Fatalf("Clock() location = %v, want UTC", got.Location())
	}
}

func TestRuntimeDepsClockUsesInjectedClock(t *testing.T) {
	want := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
	clk := RuntimeDeps{Now: func() time.Time { return want }}.Clock()
	if got := clk(); !got.Equal(want) {
		t.Fatalf("Clock() = %v, want %v", got, want)
	}
}

func TestRuntimeOwnsDefaultedClockState(t *testing.T) {
	want := time.Date(2026, 5, 5, 10, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	runtime := NewRuntime(RuntimeDeps{Now: func() time.Time { return want }})
	if got := runtime.Now(); !got.Equal(want.UTC()) || got.Location() != time.UTC {
		t.Fatalf("Now() = %v (%v), want %v UTC", got, got.Location(), want.UTC())
	}
}

func TestServiceImplDoesNotImportRootPackage(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list internal/serviceimpl: %v", err)
	}

	var pkg struct {
		Imports []string
		Deps    []string
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("decode go list output: %v", err)
	}

	const root = "github.com/easel/fizeau"
	for _, imp := range pkg.Imports {
		if imp == root {
			t.Fatalf("internal/serviceimpl imports root package %q", root)
		}
	}
	for _, dep := range pkg.Deps {
		if dep == root {
			t.Fatalf("internal/serviceimpl depends on root package %q", root)
		}
	}
}
