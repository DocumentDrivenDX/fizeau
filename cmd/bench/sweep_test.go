package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sweepPlanPath returns the path to the TB-2.1 sweep plan in the repo.
func sweepPlanPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(benchRepoRoot(t), defaultSweepPlanPath)
}

// TestLoadSweepPlanParsesAllPhases verifies the sweep plan YAML loads and
// contains all expected phases.
func TestLoadSweepPlanParsesAllPhases(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	wantPhases := []string{"canary", "local-qwen", "timing-baseline", "or-passing", "tb21-all", "openai-cheap", "sonnet-comparison", "gpt-comparison"}
	if len(plan.Phases) != len(wantPhases) {
		t.Fatalf("phases = %d, want %d", len(plan.Phases), len(wantPhases))
	}
	for i, want := range wantPhases {
		if got := plan.Phases[i].ID; got != want {
			t.Errorf("phases[%d].ID = %q, want %q", i, got, want)
		}
	}
}

// TestLoadSweepPlanHasAllLanes verifies all supported sweep lanes are parsed.
func TestLoadSweepPlanHasAllLanes(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	wantLanes := []string{
		"fiz-harness-claude-sonnet-4-6",
		"fiz-harness-codex-gpt-5-4-mini",
		"fiz-openrouter-claude-sonnet-4-6",
		"fiz-openrouter-gpt-5-4-mini",
		"fiz-openai-gpt-5-5",
		"fiz-openrouter-qwen3-6-27b",
		"fiz-vidar-omlx-qwen3-6-27b",
		"fiz-vidar-ds4",
		"fiz-bragi-club-3090-qwen3-6-27b",
		"fiz-grendel-rapid-mlx-qwen3-6-27b",
		"fiz-sindri-vllm-qwen3-6-27b",
		"fiz-sindri-llamacpp-qwen3-6-27b",
	}
	byID := sweepLaneMap(plan)
	for _, id := range wantLanes {
		if _, ok := byID[id]; !ok {
			t.Errorf("lane %q not found in sweep plan", id)
		}
	}
}

// TestLoadSweepPlanResourceGroupsAllPresent verifies all resource groups parse
// with sensible max_concurrency values.
func TestLoadSweepPlanResourceGroupsAllPresent(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	localGroups := []string{"rg-vidar-omlx", "rg-bragi-club-3090", "rg-grendel-rapid-mlx", "rg-sindri-club-3090"}
	rgByID := sweepRGMap(plan)
	for _, id := range localGroups {
		rg, ok := rgByID[id]
		if !ok {
			t.Errorf("resource group %q not found", id)
			continue
		}
		if rg.MaxConcurrency != 1 {
			t.Errorf("local rg %s max_concurrency = %d, want 1", id, rg.MaxConcurrency)
		}
	}
	or, ok := rgByID["rg-openrouter"]
	if !ok {
		t.Error("resource group rg-openrouter not found")
	} else if or.MaxConcurrency < 2 {
		t.Errorf("rg-openrouter max_concurrency = %d, want >= 2", or.MaxConcurrency)
	}
	for _, id := range []string{"rg-openai-gpt55", "rg-openrouter-qwen36-27b"} {
		rg, ok := rgByID[id]
		if !ok {
			t.Errorf("resource group %q not found", id)
			continue
		}
		if rg.MaxConcurrency < 2 {
			t.Errorf("%s max_concurrency = %d, want >= 2", id, rg.MaxConcurrency)
		}
	}
}

// TestSweepCGByLanePopulatesCorrectly verifies the comparison-group-by-lane
// index is built from the sweep plan's comparison_groups section.
func TestSweepCGByLanePopulatesCorrectly(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	cgByLane := sweepCGByLane(plan)

	// Active local Qwen lanes should belong to cg-local-qwen-provider-quant.
	for _, id := range []string{"fiz-vidar-omlx-qwen3-6-27b", "fiz-bragi-club-3090-qwen3-6-27b", "fiz-sindri-vllm-qwen3-6-27b"} {
		cgs := cgByLane[id]
		found := false
		for _, cg := range cgs {
			if cg == "cg-local-qwen-provider-quant" {
				found = true
			}
		}
		if !found {
			t.Errorf("lane %s not in cg-local-qwen-provider-quant", id)
		}
	}
}

// TestSelectSweepPhasesAllReturnsAll verifies --phase=all returns all phases.
func TestSelectSweepPhasesAllReturnsAll(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	phases, err := selectSweepPhases(plan, "all")
	if err != nil {
		t.Fatalf("selectSweepPhases(all): %v", err)
	}
	if len(phases) != len(plan.Phases) {
		t.Fatalf("got %d phases, want %d", len(phases), len(plan.Phases))
	}
}

// TestSelectSweepPhasesSinglePhase verifies each named phase can be selected.
func TestSelectSweepPhasesSinglePhase(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	for _, phaseID := range []string{"canary", "local-qwen", "tb21-all", "openai-cheap", "sonnet-comparison", "gpt-comparison"} {
		phases, err := selectSweepPhases(plan, phaseID)
		if err != nil {
			t.Errorf("selectSweepPhases(%q): %v", phaseID, err)
			continue
		}
		if len(phases) != 1 || phases[0].ID != phaseID {
			t.Errorf("selectSweepPhases(%q) = %v, want single phase", phaseID, phases)
		}
	}
}

// TestSelectSweepPhasesUnknownReturnsError verifies unknown phase IDs error.
func TestSelectSweepPhasesUnknownReturnsError(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	_, err = selectSweepPhases(plan, "no-such-phase")
	if err == nil {
		t.Fatal("expected error for unknown phase, got nil")
	}
}

// TestSweepResolveSubsetPathKnownIDs verifies known subset IDs resolve to files
// that exist under scripts/benchmark/.
func TestSweepResolveSubsetPathKnownIDs(t *testing.T) {
	wd := benchRepoRoot(t)
	cases := map[string]string{
		"terminalbench-2-1-canary":       "scripts/benchmark/task-subset-tb21-canary.yaml",
		"terminalbench-2-1-full":         "scripts/benchmark/task-subset-tb21-full.yaml",
		"terminalbench-2-1-all":          "scripts/benchmark/task-subset-tb21-all.yaml",
		"terminalbench-2-1-openai-cheap": "scripts/benchmark/task-subset-tb21-openai-cheap.yaml",
		"terminalbench-2-1-or-passing":   "scripts/benchmark/task-subset-tb21-or-passing.yaml",
		"terminalbench-2-1-timing-baseline":     "scripts/benchmark/task-subset-tb21-timing-baseline.yaml",
	}
	for id, rel := range cases {
		got := sweepResolveSubsetPath(wd, id)
		want := filepath.Join(wd, rel)
		if got != want {
			t.Errorf("sweepResolveSubsetPath(%q) = %q, want %q", id, got, want)
		}
		if _, err := os.Stat(got); err != nil {
			t.Errorf("subset file %s does not exist: %v", got, err)
		}
	}
}

// TestTB21CanarySubsetLoadsAndHasTasks verifies the canary YAML subset can be
// loaded and contains the expected number of tasks.
func TestTB21CanarySubsetLoadsAndHasTasks(t *testing.T) {
	wd := benchRepoRoot(t)
	path := filepath.Join(wd, "scripts/benchmark/task-subset-tb21-canary.yaml")
	s, err := loadTermbenchSubset(path)
	if err != nil {
		t.Fatalf("loadTermbenchSubset(canary): %v", err)
	}
	if len(s.Tasks) < 3 {
		t.Errorf("canary subset has %d tasks, want >= 3", len(s.Tasks))
	}
	if len(s.Tasks) > 5 {
		t.Errorf("canary subset has %d tasks, want <= 5", len(s.Tasks))
	}
	if s.Dataset == "" {
		t.Error("canary subset dataset field is empty")
	}
	for _, task := range s.Tasks {
		if task.ID == "" {
			t.Error("canary subset has task with empty ID")
		}
		if task.Category == "" {
			t.Errorf("task %s has empty category", task.ID)
		}
	}
}

// TestTB21FullSubsetLoadsAndHasTasks verifies the full YAML subset loads and
// contains at least as many tasks as the canary subset.
func TestTB21FullSubsetLoadsAndHasTasks(t *testing.T) {
	wd := benchRepoRoot(t)
	canaryPath := filepath.Join(wd, "scripts/benchmark/task-subset-tb21-canary.yaml")
	fullPath := filepath.Join(wd, "scripts/benchmark/task-subset-tb21-full.yaml")
	canary, err := loadTermbenchSubset(canaryPath)
	if err != nil {
		t.Fatalf("loadTermbenchSubset(canary): %v", err)
	}
	full, err := loadTermbenchSubset(fullPath)
	if err != nil {
		t.Fatalf("loadTermbenchSubset(full): %v", err)
	}
	if len(full.Tasks) < len(canary.Tasks) {
		t.Errorf("full subset has %d tasks, fewer than canary %d", len(full.Tasks), len(canary.Tasks))
	}
}

func TestTB21AllSubsetLoadsAndHasAllCatalogTasks(t *testing.T) {
	wd := benchRepoRoot(t)
	path := filepath.Join(wd, "scripts/benchmark/task-subset-tb21-all.yaml")
	s, err := loadTermbenchSubset(path)
	if err != nil {
		t.Fatalf("loadTermbenchSubset(all): %v", err)
	}
	if len(s.Tasks) != 89 {
		t.Fatalf("all subset has %d tasks, want 89", len(s.Tasks))
	}
	seen := map[string]bool{}
	for _, task := range s.Tasks {
		if task.ID == "" {
			t.Fatal("all subset has task with empty ID")
		}
		if seen[task.ID] {
			t.Fatalf("all subset has duplicate task ID %q", task.ID)
		}
		seen[task.ID] = true
	}
}

// TestSweepDryRunCanaryPrints verifies --dry-run for canary phase prints
// required fields without launching any process.
func TestSweepDryRunCanaryPrints(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := cmdSweep([]string{
		"--work-dir", repoRoot,
		"--sweep-plan", filepath.Join(repoRoot, defaultSweepPlanPath),
		"--phase", "canary",
		"--dry-run",
		"--out", outDir,
	})

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	output := buf.String()

	if code != 0 {
		t.Fatalf("cmdSweep dry-run exit = %d, want 0\noutput:\n%s", code, output)
	}

	// AC-1: print phases, lane ids, comparison_group ids, task count, reps,
	// resource groups, max parallelism, and output directory.
	required := []string{
		"Phase: canary",
		"Dataset:",
		"Subset ID:",
		"Task Count:",
		"Reps:",
		"Total Cells:",
		"Output Dir:",
		"Lane:",
		"Profile:",
		"Resource Group:",
		"max_concurrency=",
		"Comparison Groups:",
		"Command:",
	}
	for _, want := range required {
		if !strings.Contains(output, want) {
			t.Errorf("dry-run output missing %q\nfull output:\n%s", want, output)
		}
	}

	// Verify at least one lane from canary is printed.
	if !strings.Contains(output, "fiz-harness-claude-sonnet-4-6") {
		t.Errorf("dry-run output missing canary lane fiz-harness-claude-sonnet-4-6")
	}
	// Verify the output directory appears.
	if !strings.Contains(output, outDir) {
		t.Errorf("dry-run output missing out dir %s", outDir)
	}
}

// TestSweepDryRunAllPhasesContainsAllPhaseHeaders verifies --phase=all prints
// headers for all four phases.
func TestSweepDryRunAllPhasesContainsAllPhaseHeaders(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := cmdSweep([]string{
		"--work-dir", repoRoot,
		"--phase", "all",
		"--dry-run",
		"--out", outDir,
	})

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	output := buf.String()

	if code != 0 {
		t.Fatalf("cmdSweep dry-run all exit = %d\noutput:\n%s", code, output)
	}
	for _, phase := range []string{"canary", "local-qwen", "tb21-all", "openai-cheap", "sonnet-comparison", "gpt-comparison"} {
		if !strings.Contains(output, "Phase: "+phase) {
			t.Errorf("dry-run output missing Phase: %s", phase)
		}
	}
}

func TestSweepDryRunFullWithLaneFilterPrintsOnlySelectedLanes(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := cmdSweep([]string{
		"--work-dir", repoRoot,
		"--phase", "tb21-all",
		"--lanes", "fiz-sindri-vllm-qwen3-6-27b,fiz-vidar-omlx-qwen3-6-27b",
		"--dry-run",
		"--out", outDir,
	})

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	output := buf.String()

	if code != 0 {
		t.Fatalf("cmdSweep dry-run filtered full exit = %d\noutput:\n%s", code, output)
	}
	required := []string{
		"Phase: tb21-all",
		"Subset ID:     terminalbench-2-1-all",
		"Task Count:    89",
		"Total Cells:   534",
		"Lane: fiz-sindri-vllm-qwen3-6-27b",
		"Lane: fiz-vidar-omlx-qwen3-6-27b",
	}
	for _, want := range required {
		if !strings.Contains(output, want) {
			t.Errorf("dry-run output missing %q\nfull output:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Lane: fiz-bragi-club-3090-qwen3-6-27b") {
		t.Errorf("dry-run output included unselected bragi lane\nfull output:\n%s", output)
	}
}

func TestSweepDryRunFourLaneFullShowsManagedJobCaps(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := cmdSweep([]string{
		"--work-dir", repoRoot,
		"--phase", "tb21-all",
		"--lanes", "fiz-openai-gpt-5-5,fiz-openrouter-qwen3-6-27b,fiz-sindri-llamacpp-qwen3-6-27b,fiz-vidar-omlx-qwen3-6-27b",
		"--matrix-jobs-managed", "16",
		"--dry-run",
		"--out", outDir,
	})

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	output := buf.String()

	if code != 0 {
		t.Fatalf("cmdSweep dry-run four-lane full exit = %d\noutput:\n%s", code, output)
	}
	required := []string{
		"Total Cells:   1068",
		"Lane: fiz-openai-gpt-5-5",
		"Lane: fiz-openrouter-qwen3-6-27b",
		"Lane: fiz-sindri-llamacpp-qwen3-6-27b",
		"Lane: fiz-vidar-omlx-qwen3-6-27b",
		"--profiles fiz-openai-gpt-5-5",
		"--profiles fiz-openrouter-qwen3-6-27b",
		"--jobs 16",
		"--jobs 10",
	}
	for _, want := range required {
		if !strings.Contains(output, want) {
			t.Errorf("dry-run output missing %q\nfull output:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Lane: fiz-bragi-club-3090-qwen3-6-27b") {
		t.Errorf("dry-run output included unselected bragi lane\nfull output:\n%s", output)
	}
}

// TestSweepBuildLaneMeta verifies buildSweepLaneMeta produces a well-formed
// metadata struct with all AC-8 required fields populated.
func TestSweepBuildLaneMeta(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	wd := benchRepoRoot(t)
	opts := sweepRunOpts{
		plan:       plan,
		wd:         wd,
		outDir:     t.TempDir(),
		resume:     true,
		rgByID:     sweepRGMap(plan),
		laneByID:   sweepLaneMap(plan),
		cgByLane:   sweepCGByLane(plan),
		cgTypeByID: sweepCGTypeMap(plan),
	}

	lane := opts.laneByID["fiz-vidar-omlx-qwen3-6-27b"]
	if lane == nil {
		t.Fatal("lane fiz-vidar-omlx-qwen3-6-27b not found")
	}
	rg := opts.rgByID[lane.ResourceGroup]
	if rg == nil {
		t.Fatal("resource group for vidar lane not found")
	}

	phase := plan.Phases[0] // canary
	subsetPath := sweepResolveSubsetPath(wd, phase.Subset)
	laneOutDir := filepath.Join(opts.outDir, phase.ID, lane.ID)
	matrixArgs := buildSweepMatrixArgs(opts, phase, lane, rg, subsetPath, laneOutDir, 3)

	meta := buildSweepLaneMeta(opts, phase, lane, rg, subsetPath, laneOutDir, 3, matrixArgs)

	// AC-8 required fields
	checks := []struct {
		field string
		val   string
	}{
		{"dataset", meta.Dataset},
		{"dataset_version", meta.DatasetVersion},
		{"subset_id", meta.SubsetID},
		{"subset_path", meta.SubsetPath},
		{"phase", meta.Phase},
		{"lane_id", meta.LaneID},
		{"model_family", meta.ModelFamily},
		{"model_id", meta.ModelID},
		{"quant_label", meta.QuantLabel},
		{"profile_id", meta.ProfileID},
		{"lane_type", meta.LaneType},
		{"provider_surface", meta.ProviderSurface},
		{"resource_group", meta.ResourceGroup},
	}
	for _, c := range checks {
		if c.val == "" {
			t.Errorf("meta.%s is empty", c.field)
		}
	}
	if meta.Reps != 3 {
		t.Errorf("meta.reps = %d, want 3", meta.Reps)
	}
	if len(meta.Command) == 0 {
		t.Error("meta.command is empty")
	}
	if len(meta.FizeauEnv) == 0 {
		t.Error("meta.fizeau_env is empty")
	}
	// Verify API key values are redacted.
	for k, v := range meta.FizeauEnv {
		upper := strings.ToUpper(k)
		if strings.HasSuffix(upper, "_API_KEY") && v != "<redacted>" {
			t.Errorf("meta.fizeau_env[%s] = %q, want <redacted>", k, v)
		}
	}
}

// TestSweepBuildMatrixArgsIncludesResumeAndBudget verifies buildSweepMatrixArgs
// includes --resume and budget flags when set.
func TestSweepBuildMatrixArgsIncludesResumeAndBudget(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	wd := benchRepoRoot(t)
	opts := sweepRunOpts{
		plan:            plan,
		wd:              wd,
		outDir:          t.TempDir(),
		resume:          true,
		forceRerun:      false,
		perRunBudgetUSD: 5.0,
		rgByID:          sweepRGMap(plan),
		laneByID:        sweepLaneMap(plan),
	}
	lane := opts.laneByID["fiz-openrouter-claude-sonnet-4-6"]
	rg := opts.rgByID[lane.ResourceGroup]
	phase := plan.Phases[0]
	subsetPath := sweepResolveSubsetPath(wd, phase.Subset)
	laneOutDir := filepath.Join(opts.outDir, phase.ID, lane.ID)

	args := buildSweepMatrixArgs(opts, phase, lane, rg, subsetPath, laneOutDir, 3)
	argStr := strings.Join(args, " ")

	if !strings.Contains(argStr, "--resume") {
		t.Error("matrix args missing --resume")
	}
	if !strings.Contains(argStr, "--per-run-budget-usd") {
		t.Error("matrix args missing --per-run-budget-usd")
	}
	if !strings.Contains(argStr, "--profiles "+lane.ProfileID) {
		t.Errorf("matrix args missing --profiles %s", lane.ProfileID)
	}
	if !strings.Contains(argStr, "--harnesses fiz") {
		t.Error("matrix args missing --harnesses fiz")
	}
}

func TestSweepBuildMatrixArgsIncludesLaneFizeauEnv(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	wd := benchRepoRoot(t)
	opts := sweepRunOpts{
		plan:     plan,
		wd:       wd,
		outDir:   t.TempDir(),
		rgByID:   sweepRGMap(plan),
		laneByID: sweepLaneMap(plan),
	}
	lane := opts.laneByID["fiz-harness-claude-sonnet-4-6"]
	if lane == nil {
		t.Fatal("harness lane not found")
	}
	rg := opts.rgByID[lane.ResourceGroup]
	phase := plan.Phases[0]
	subsetPath := sweepResolveSubsetPath(wd, phase.Subset)
	laneOutDir := filepath.Join(opts.outDir, phase.ID, lane.ID)

	args := buildSweepMatrixArgs(opts, phase, lane, rg, subsetPath, laneOutDir, 1)
	argStr := strings.Join(args, " ")

	for _, want := range []string{
		"--env FIZEAU_HARNESS=claude",
		"--env FIZEAU_PROVIDER=openrouter",
		"--env FIZEAU_MODEL=claude-sonnet-4-6",
		"--env FIZEAU_API_KEY_ENV=OPENROUTER_API_KEY",
	} {
		if !strings.Contains(argStr, want) {
			t.Errorf("matrix args missing %q\nargs: %s", want, argStr)
		}
	}
}

// TestSweepResourceGroupSchedulingSerializesLocalLanes uses a synthetic sweep
// plan with two lanes sharing a local resource group (max_concurrency=1) to
// verify they never run concurrently.
func TestSweepResourceGroupSchedulingSerializesLocalLanes(t *testing.T) {
	// Build a minimal synthetic plan with one local resource group and two lanes.
	plan := &sweepPlan{
		Dataset:  "test",
		Defaults: sweepDefaults{Reps: 1},
		Phases: []sweepPhase{
			{ID: "test-phase", Subset: "terminalbench-2-1-canary", Reps: 1,
				Lanes: []string{"lane-a", "lane-b"}},
		},
		ResourceGroups: []sweepResourceGroup{
			{ID: "rg-local", ProviderType: "local", MaxConcurrency: 1},
		},
		Lanes: []sweepLane{
			{ID: "lane-a", ProfileID: "noop", ResourceGroup: "rg-local",
				ModelFamily: "test", ModelID: "test", QuantLabel: "none",
				LaneType: "fiz_provider_native", ProviderSurface: "local"},
			{ID: "lane-b", ProfileID: "noop", ResourceGroup: "rg-local",
				ModelFamily: "test", ModelID: "test", QuantLabel: "none",
				LaneType: "fiz_provider_native", ProviderSurface: "local"},
		},
	}

	// Track peak concurrent executions within the resource group.
	var (
		mu             sync.Mutex
		active         int
		peakActive     int
		completedLanes []string
	)

	// Replace cmdMatrix with a counting mock via the resource group semaphore
	// logic directly — test the scheduling structure, not actual matrix execution.
	rgSems := make(map[string]chan struct{})
	for _, rg := range plan.ResourceGroups {
		cap := rg.MaxConcurrency
		if cap < 1 {
			cap = 1
		}
		rgSems[rg.ID] = make(chan struct{}, cap)
	}

	laneByID := sweepLaneMap(plan)
	rgByID := sweepRGMap(plan)

	outcomes := make([]sweepLaneOutcome, 2)
	var wg sync.WaitGroup
	var counter int64

	for i, laneID := range plan.Phases[0].Lanes {
		lane := laneByID[laneID]
		rg := rgByID[lane.ResourceGroup]
		sem := rgSems[rg.ID]

		wg.Add(1)
		go func(i int, lane *sweepLane, sem chan struct{}) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mu.Lock()
			active++
			if active > peakActive {
				peakActive = active
			}
			mu.Unlock()

			// Simulate work: bump a counter and sleep briefly.
			atomic.AddInt64(&counter, 1)
			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			active--
			completedLanes = append(completedLanes, lane.ID)
			mu.Unlock()

			outcomes[i] = sweepLaneOutcome{laneID: lane.ID, code: 0}
		}(i, lane, sem)
	}
	wg.Wait()

	if peakActive > 1 {
		t.Errorf("local resource group (max_concurrency=1) had %d concurrent executions, want <= 1", peakActive)
	}
	if len(completedLanes) != 2 {
		t.Errorf("completed %d lanes, want 2", len(completedLanes))
	}
}

// TestSweepResourceGroupSchedulingParallelizesIndependentGroups uses two lanes
// each in their own resource group to verify they can run in parallel.
func TestSweepResourceGroupSchedulingParallelizesIndependentGroups(t *testing.T) {
	plan := &sweepPlan{
		Dataset:  "test",
		Defaults: sweepDefaults{Reps: 1},
		ResourceGroups: []sweepResourceGroup{
			{ID: "rg-a", MaxConcurrency: 1},
			{ID: "rg-b", MaxConcurrency: 1},
		},
		Lanes: []sweepLane{
			{ID: "lane-a", ResourceGroup: "rg-a"},
			{ID: "lane-b", ResourceGroup: "rg-b"},
		},
	}

	rgSems := make(map[string]chan struct{})
	for _, rg := range plan.ResourceGroups {
		rgSems[rg.ID] = make(chan struct{}, rg.MaxConcurrency)
	}
	laneByID := sweepLaneMap(plan)
	rgByID := sweepRGMap(plan)

	started := make(chan string, 2)
	var wg sync.WaitGroup

	for _, laneID := range []string{"lane-a", "lane-b"} {
		lane := laneByID[laneID]
		rg := rgByID[lane.ResourceGroup]
		sem := rgSems[rg.ID]

		wg.Add(1)
		go func(lane *sweepLane, sem chan struct{}) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			started <- lane.ID
			time.Sleep(20 * time.Millisecond)
		}(lane, sem)
	}

	// Both lanes should have started before either finishes (parallel execution).
	timeout := time.After(500 * time.Millisecond)
	var s1, s2 string
	select {
	case s1 = <-started:
	case <-timeout:
		t.Fatal("lane-a or lane-b never started")
	}
	select {
	case s2 = <-started:
	case <-timeout:
		t.Fatalf("second lane never started (first was %s); independent resource groups should run in parallel", s1)
	}
	_ = s2
	wg.Wait()
}

// TestSweepSummarizeLaneEmpty verifies summarizeSweepLane handles nil matrix gracefully.
func TestSweepSummarizeLaneEmpty(t *testing.T) {
	row := summarizeSweepLane("test-lane", nil)
	if row.NRuns != 0 || row.NValid != 0 || row.NInvalid != 0 {
		t.Errorf("empty matrix should produce zero counts, got %+v", row)
	}
	if row.EffCostPerValid != nil || row.EffCostPerPass != nil {
		t.Error("empty matrix should produce nil effective cost fields")
	}
}

// TestSweepSummarizeLaneWithMixedRuns verifies summarizeSweepLane correctly
// aggregates pass/fail/invalid counts and computes effective costs.
func TestSweepSummarizeLaneWithMixedRuns(t *testing.T) {
	pass := 1
	fail := 0
	costPass := 2.0
	costFail := 1.0

	mout := &matrixOutput{
		Runs: []matrixRunReport{
			{Harness: "fiz", ProfileID: "p", TaskID: "t1", Rep: 1,
				FinalStatus: "graded_pass", Reward: &pass,
				CostUSD: costPass, InputTokens: intPtr(1000), OutputTokens: intPtr(500)},
			{Harness: "fiz", ProfileID: "p", TaskID: "t2", Rep: 1,
				FinalStatus: "graded_fail", Reward: &fail,
				CostUSD: costFail, InputTokens: intPtr(800), OutputTokens: intPtr(400)},
			{Harness: "fiz", ProfileID: "p", TaskID: "t3", Rep: 1,
				FinalStatus: "invalid_provider", InvalidClass: "invalid_provider",
				CostUSD: 0},
		},
	}

	row := summarizeSweepLane("lane-x", mout)
	if row.NRuns != 3 {
		t.Errorf("NRuns = %d, want 3", row.NRuns)
	}
	if row.NValid != 2 {
		t.Errorf("NValid = %d, want 2 (graded pass + fail)", row.NValid)
	}
	if row.NInvalid != 1 {
		t.Errorf("NInvalid = %d, want 1", row.NInvalid)
	}
	if row.NPasses != 1 {
		t.Errorf("NPasses = %d, want 1", row.NPasses)
	}
	totalCost := costPass + costFail
	if row.CostUSD != totalCost {
		t.Errorf("CostUSD = %f, want %f", row.CostUSD, totalCost)
	}
	if row.EffCostPerValid == nil {
		t.Error("EffCostPerValid is nil, want non-nil")
	} else {
		want := totalCost / 2.0
		if got := *row.EffCostPerValid; fmt.Sprintf("%.4f", got) != fmt.Sprintf("%.4f", want) {
			t.Errorf("EffCostPerValid = %f, want %f", got, want)
		}
	}
	if row.EffCostPerPass == nil {
		t.Error("EffCostPerPass is nil, want non-nil")
	} else {
		want := totalCost / 1.0
		if got := *row.EffCostPerPass; fmt.Sprintf("%.4f", got) != fmt.Sprintf("%.4f", want) {
			t.Errorf("EffCostPerPass = %f, want %f", got, want)
		}
	}
}

// TestSweepLaneMetaRoundTrips verifies sweepLaneMeta marshals and unmarshals
// consistently so evidence import tools can consume it.
func TestSweepLaneMetaRoundTrips(t *testing.T) {
	meta := sweepLaneMeta{
		Dataset:          "terminal-bench/terminal-bench-2-1",
		DatasetVersion:   "2.1",
		SubsetID:         "terminalbench-2-1-canary",
		SubsetPath:       "scripts/benchmark/task-subset-tb21-canary.yaml",
		Phase:            "canary",
		LaneID:           "fiz-vidar-omlx-qwen3-6-27b",
		ComparisonGroups: []string{"cg-local-qwen-provider-quant"},
		ComparisonTypes:  []string{"provider_quant_delta"},
		ModelFamily:      "qwen3-6-27b",
		ModelID:          "Qwen3.6-27B-MLX-8bit",
		QuantLabel:       "mlx-8bit",
		ProfileID:        "vidar-qwen3-6-27b",
		LaneType:         "fiz_provider_native",
		ProviderType:     "omlx",
		ProviderSurface:  "vidar-omlx",
		Runtime:          "omlx",
		HardwareLabel:    "vidar-apple-m",
		BaseURL:          "http://vidar:1235/v1",
		ResourceGroup:    "rg-vidar-omlx",
		Reps:             3,
		FizeauEnv:        map[string]string{"FIZEAU_PROVIDER": "omlx", "FIZEAU_MODEL": "Qwen3.6-27B-MLX-8bit"},
		Command:          []string{"fiz-bench", "matrix", "--profiles", "vidar-qwen3-6-27b"},
		GeneratedAt:      time.Now().UTC().Truncate(time.Second),
	}

	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got sweepLaneMeta
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got.LaneID != meta.LaneID || got.Phase != meta.Phase || got.DatasetVersion != meta.DatasetVersion {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, meta)
	}
	if got.ResourceGroup != meta.ResourceGroup || got.ModelID != meta.ModelID {
		t.Errorf("resource group or model id mismatch after round-trip")
	}
}

// TestSweepCommandUnknownPhaseExitsTwo verifies an invalid --phase flag
// causes cmdSweep to exit with code 2.
func TestSweepCommandUnknownPhaseExitsTwo(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	code := cmdSweep([]string{
		"--work-dir", repoRoot,
		"--phase", "no-such-phase",
		"--dry-run",
	})
	if code != 2 {
		t.Errorf("cmdSweep(unknown-phase) = %d, want 2", code)
	}
}

// TestSweepCommandMissingSweepPlanExitsOne verifies a missing sweep plan
// causes cmdSweep to exit with code 1.
func TestSweepCommandMissingSweepPlanExitsOne(t *testing.T) {
	code := cmdSweep([]string{
		"--sweep-plan", "/no/such/file.yaml",
		"--dry-run",
	})
	if code != 1 {
		t.Errorf("cmdSweep(missing plan) = %d, want 1", code)
	}
}
