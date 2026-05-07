package fizeau_test

import (
	"context"
	"path/filepath"
	"testing"

	fizeau "github.com/DocumentDrivenDX/fizeau"
)

func TestListHarnesses_shape(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fakeHome, ".config"))
	svc, err := fizeau.New(fizeau.ServiceOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	list, err := svc.ListHarnesses(context.Background())
	if err != nil {
		t.Fatalf("ListHarnesses: %v", err)
	}

	if len(list) == 0 {
		t.Fatal("expected at least one harness")
	}

	// Index by name for targeted assertions.
	byName := make(map[string]fizeau.HarnessInfo, len(list))
	for _, h := range list {
		if h.Name == "" {
			t.Errorf("harness with empty Name found")
		}
		byName[h.Name] = h
	}

	// All builtins must appear.
	expected := []string{
		"codex", "claude", "gemini", "opencode", "fiz",
		"pi", "virtual", "script", "openrouter", "lmstudio", "omlx",
	}
	for _, name := range expected {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing harness %q", name)
		}
	}

	t.Run("codex", func(t *testing.T) {
		h := byName["codex"]
		assertContains(t, h.SupportedPermissions, "safe", "codex permissions")
		assertContains(t, h.SupportedPermissions, "supervised", "codex permissions")
		assertContains(t, h.SupportedPermissions, "unrestricted", "codex permissions")
		assertContains(t, h.SupportedReasoning, "low", "codex reasoning")
		assertContains(t, h.SupportedReasoning, "medium", "codex reasoning")
		assertContains(t, h.SupportedReasoning, "high", "codex reasoning")
		assertContains(t, h.SupportedReasoning, "xhigh", "codex reasoning")
		assertContains(t, h.SupportedReasoning, "max", "codex reasoning")
		if h.CostClass != "medium" {
			t.Errorf("codex CostClass: want medium, got %q", h.CostClass)
		}
		if h.IsSubscription != true {
			t.Errorf("codex IsSubscription: want true")
		}
		if !h.AutoRoutingEligible {
			t.Errorf("codex AutoRoutingEligible: want true")
		}
		if h.IsLocal {
			t.Errorf("codex IsLocal: want false")
		}
		if h.Type != "subprocess" {
			t.Errorf("codex Type: want subprocess, got %q", h.Type)
		}
		if h.DefaultModel != "gpt-5.4" {
			t.Errorf("codex DefaultModel: want gpt-5.4, got %q", h.DefaultModel)
		}
	})

	t.Run("claude", func(t *testing.T) {
		h := byName["claude"]
		assertContains(t, h.SupportedPermissions, "safe", "claude permissions")
		assertContains(t, h.SupportedPermissions, "unrestricted", "claude permissions")
		assertContains(t, h.SupportedReasoning, "low", "claude reasoning")
		assertContains(t, h.SupportedReasoning, "high", "claude reasoning")
		assertContains(t, h.SupportedReasoning, "xhigh", "claude reasoning")
		assertContains(t, h.SupportedReasoning, "max", "claude reasoning")
		if h.CostClass != "medium" {
			t.Errorf("claude CostClass: want medium, got %q", h.CostClass)
		}
		if h.IsSubscription != true {
			t.Errorf("claude IsSubscription: want true")
		}
		if !h.AutoRoutingEligible {
			t.Errorf("claude AutoRoutingEligible: want true")
		}
		if h.Type != "subprocess" {
			t.Errorf("claude Type: want subprocess, got %q", h.Type)
		}
		if h.DefaultModel != "claude-sonnet-4-6" {
			t.Errorf("claude DefaultModel: want claude-sonnet-4-6, got %q", h.DefaultModel)
		}
		// Quota may be nil (no cache on CI); just check it doesn't panic.
		_ = h.Quota
	})

	t.Run("fiz_native", func(t *testing.T) {
		h := byName["fiz"]
		if h.Type != "native" {
			t.Errorf("fiz Type: want native, got %q", h.Type)
		}
		if !h.IsLocal {
			t.Errorf("fiz IsLocal: want true")
		}
		if !h.AutoRoutingEligible {
			t.Errorf("fiz AutoRoutingEligible: want true")
		}
		if h.CostClass != "local" {
			t.Errorf("fiz CostClass: want local, got %q", h.CostClass)
		}
		if !h.Available {
			t.Errorf("fiz Available: want true (embedded)")
		}
		if h.DefaultModel != "" {
			t.Errorf("agent DefaultModel: want empty, got %q", h.DefaultModel)
		}
		assertContains(t, h.SupportedPermissions, "safe", "agent permissions")
		assertContains(t, h.SupportedPermissions, "unrestricted", "agent permissions")
		assertContains(t, h.SupportedReasoning, "low", "agent reasoning")
		assertContains(t, h.SupportedReasoning, "medium", "agent reasoning")
		assertContains(t, h.SupportedReasoning, "high", "agent reasoning")
	})

	t.Run("openrouter_native", func(t *testing.T) {
		h := byName["openrouter"]
		if h.Type != "native" {
			t.Errorf("openrouter Type: want native, got %q", h.Type)
		}
		if h.CostClass != "medium" {
			t.Errorf("openrouter CostClass: want medium, got %q", h.CostClass)
		}
	})

	t.Run("lmstudio_local", func(t *testing.T) {
		h := byName["lmstudio"]
		if h.CostClass != "local" {
			t.Errorf("lmstudio CostClass: want local, got %q", h.CostClass)
		}
		if !h.IsLocal {
			t.Errorf("lmstudio IsLocal: want true")
		}
	})

	t.Run("omlx_local", func(t *testing.T) {
		h := byName["omlx"]
		if h.Type != "native" {
			t.Errorf("omlx Type: want native, got %q", h.Type)
		}
		if h.CostClass != "local" {
			t.Errorf("omlx CostClass: want local, got %q", h.CostClass)
		}
		if !h.IsLocal {
			t.Errorf("omlx IsLocal: want true")
		}
	})

	t.Run("gemini_secondary_until_quota_probe_exists", func(t *testing.T) {
		h := byName["gemini"]
		if h.AutoRoutingEligible {
			t.Errorf("gemini AutoRoutingEligible: want false until quota evidence exists")
		}
		if h.CostClass != "medium" {
			t.Errorf("gemini CostClass: want medium, got %q", h.CostClass)
		}
		if h.DefaultModel != "gemini-2.5-flash" {
			t.Errorf("gemini DefaultModel: want gemini-2.5-flash, got %q", h.DefaultModel)
		}
		assertContains(t, h.SupportedPermissions, "safe", "gemini permissions")
		assertContains(t, h.SupportedPermissions, "supervised", "gemini permissions")
		assertContains(t, h.SupportedPermissions, "unrestricted", "gemini permissions")
	})

	t.Run("opencode_permissions_all_levels", func(t *testing.T) {
		h := byName["opencode"]
		if h.AutoRoutingEligible {
			t.Errorf("opencode AutoRoutingEligible: want false until cost/quota evidence exists")
		}
		if h.DefaultModel != "opencode/gpt-5.4" {
			t.Errorf("opencode DefaultModel: want opencode/gpt-5.4, got %q", h.DefaultModel)
		}
		assertContains(t, h.SupportedPermissions, "safe", "opencode permissions")
		assertContains(t, h.SupportedPermissions, "supervised", "opencode permissions")
		assertContains(t, h.SupportedPermissions, "unrestricted", "opencode permissions")
		// opencode has non-standard effort levels; only std ones count.
		assertContains(t, h.SupportedReasoning, "low", "opencode reasoning")
		assertContains(t, h.SupportedReasoning, "medium", "opencode reasoning")
		assertContains(t, h.SupportedReasoning, "high", "opencode reasoning")
		assertContains(t, h.SupportedReasoning, "minimal", "opencode reasoning")
		assertContains(t, h.SupportedReasoning, "max", "opencode reasoning")
	})

	t.Run("pi_explicit_only", func(t *testing.T) {
		h := byName["pi"]
		if h.AutoRoutingEligible {
			t.Errorf("pi AutoRoutingEligible: want false until cost/quota evidence exists")
		}
		if h.DefaultModel != "gemini-2.5-flash" {
			t.Errorf("pi DefaultModel: want gemini-2.5-flash, got %q", h.DefaultModel)
		}
		assertContains(t, h.SupportedReasoning, "minimal", "pi reasoning")
		assertContains(t, h.SupportedReasoning, "xhigh", "pi reasoning")
	})

	t.Run("capability_matrix_all_harnesses", func(t *testing.T) {
		expected := map[string]fizeau.HarnessCapabilityMatrix{
			"codex": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityOptional),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityOptional),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityOptional),
				PermissionModes: capStatus(fizeau.HarnessCapabilityOptional),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityOptional),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityOptional),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityOptional),
			},
			"claude": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityOptional),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityOptional),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityOptional),
				PermissionModes: capStatus(fizeau.HarnessCapabilityOptional),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityOptional),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityOptional),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityOptional),
			},
			"gemini": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityOptional),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityOptional),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityUnsupported),
				PermissionModes: capStatus(fizeau.HarnessCapabilityOptional),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityUnsupported),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityOptional),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityOptional),
			},
			"opencode": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityOptional),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityOptional),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityOptional),
				PermissionModes: capStatus(fizeau.HarnessCapabilityOptional),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityUnsupported),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityUnsupported),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityUnsupported),
			},
			"fiz": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityOptional),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityOptional),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityOptional),
				PermissionModes: capStatus(fizeau.HarnessCapabilityOptional),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityOptional),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityNotApplicable),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityUnsupported),
			},
			"pi": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityOptional),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityOptional),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityOptional),
				PermissionModes: capStatus(fizeau.HarnessCapabilityUnsupported),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityUnsupported),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityUnsupported),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityUnsupported),
			},
			"virtual": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityNotApplicable),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityNotApplicable),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityNotApplicable),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityNotApplicable),
				PermissionModes: capStatus(fizeau.HarnessCapabilityNotApplicable),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityNotApplicable),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityNotApplicable),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityRequired),
			},
			"script": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityNotApplicable),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityNotApplicable),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityNotApplicable),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityNotApplicable),
				PermissionModes: capStatus(fizeau.HarnessCapabilityNotApplicable),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityNotApplicable),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityNotApplicable),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityRequired),
			},
			"openrouter": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityUnsupported),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityUnsupported),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityUnsupported),
				PermissionModes: capStatus(fizeau.HarnessCapabilityUnsupported),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityUnsupported),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityUnsupported),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityUnsupported),
			},
			"lmstudio": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityUnsupported),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityUnsupported),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityUnsupported),
				PermissionModes: capStatus(fizeau.HarnessCapabilityUnsupported),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityUnsupported),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityNotApplicable),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityUnsupported),
			},
			"omlx": {
				ExecutePrompt:   capStatus(fizeau.HarnessCapabilityRequired),
				ModelDiscovery:  capStatus(fizeau.HarnessCapabilityOptional),
				ModelPinning:    capStatus(fizeau.HarnessCapabilityUnsupported),
				WorkdirContext:  capStatus(fizeau.HarnessCapabilityUnsupported),
				ReasoningLevels: capStatus(fizeau.HarnessCapabilityUnsupported),
				PermissionModes: capStatus(fizeau.HarnessCapabilityUnsupported),
				ProgressEvents:  capStatus(fizeau.HarnessCapabilityRequired),
				UsageCapture:    capStatus(fizeau.HarnessCapabilityOptional),
				FinalText:       capStatus(fizeau.HarnessCapabilityOptional),
				ToolEvents:      capStatus(fizeau.HarnessCapabilityUnsupported),
				QuotaStatus:     capStatus(fizeau.HarnessCapabilityNotApplicable),
				RecordReplay:    capStatus(fizeau.HarnessCapabilityUnsupported),
			},
		}

		for name, want := range expected {
			got := byName[name].CapabilityMatrix
			assertCapabilityMatrix(t, name, got, want)
		}
	})
}

func assertContains(t *testing.T, slice []string, want, context string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s: want %q in %v", context, want, slice)
}

func capStatus(status fizeau.HarnessCapabilityStatus) fizeau.HarnessCapability {
	return fizeau.HarnessCapability{Status: status}
}

func assertCapabilityMatrix(t *testing.T, name string, got, want fizeau.HarnessCapabilityMatrix) {
	t.Helper()
	assertCapability(t, name, "ExecutePrompt", got.ExecutePrompt, want.ExecutePrompt)
	assertCapability(t, name, "ModelDiscovery", got.ModelDiscovery, want.ModelDiscovery)
	assertCapability(t, name, "ModelPinning", got.ModelPinning, want.ModelPinning)
	assertCapability(t, name, "WorkdirContext", got.WorkdirContext, want.WorkdirContext)
	assertCapability(t, name, "ReasoningLevels", got.ReasoningLevels, want.ReasoningLevels)
	assertCapability(t, name, "PermissionModes", got.PermissionModes, want.PermissionModes)
	assertCapability(t, name, "ProgressEvents", got.ProgressEvents, want.ProgressEvents)
	assertCapability(t, name, "UsageCapture", got.UsageCapture, want.UsageCapture)
	assertCapability(t, name, "FinalText", got.FinalText, want.FinalText)
	assertCapability(t, name, "ToolEvents", got.ToolEvents, want.ToolEvents)
	assertCapability(t, name, "QuotaStatus", got.QuotaStatus, want.QuotaStatus)
	assertCapability(t, name, "RecordReplay", got.RecordReplay, want.RecordReplay)
}

func assertCapability(t *testing.T, harness, field string, got, want fizeau.HarnessCapability) {
	t.Helper()
	if got.Status != want.Status {
		t.Errorf("%s.%s Status: got %q, want %q", harness, field, got.Status, want.Status)
	}
	if got.Detail == "" {
		t.Errorf("%s.%s Detail: should explain the capability status", harness, field)
	}
}
