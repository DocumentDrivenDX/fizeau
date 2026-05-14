package fizeau

import (
	"github.com/easel/fizeau/internal/harnesses"
	claudeharness "github.com/easel/fizeau/internal/harnesses/claude"
	codexharness "github.com/easel/fizeau/internal/harnesses/codex"
	geminiharness "github.com/easel/fizeau/internal/harnesses/gemini"
	opencodeharness "github.com/easel/fizeau/internal/harnesses/opencode"
	piharness "github.com/easel/fizeau/internal/harnesses/pi"
)

// defaultHarnessInstances returns the production map of registered
// Harness implementations keyed by harness name. Only subprocess
// harnesses with concrete Runner types appear here; embedded
// ("fiz", "virtual", "script") and HTTP-only providers do not own
// quota/account state and are deliberately omitted — the scheduler
// treats absence as "no QuotaHarness/AccountHarness behavior".
//
// This file isolates the per-harness package imports from service.go so
// that service.go can drop them as each harness migrates onto the
// CONTRACT-004 sub-interface surface. The dispatcher in
// internal/serviceimpl/execute_dispatch.go is the only other
// runner-construction seam allowed by CONTRACT-004 invariant #1.
func defaultHarnessInstances() map[string]harnesses.Harness {
	return map[string]harnesses.Harness{
		"claude":   &claudeharness.Runner{},
		"codex":    &codexharness.Runner{},
		"gemini":   &geminiharness.Runner{},
		"opencode": &opencodeharness.Runner{},
		"pi":       &piharness.Runner{},
	}
}
