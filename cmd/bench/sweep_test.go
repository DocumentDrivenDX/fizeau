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

func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r) //nolint:errcheck
		close(done)
	}()

	code := fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	os.Stdout = old
	<-done

	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return code, buf.String()
}

// sweepPlanPath returns the path to the TB-2.1 sweep plan in the repo.
func sweepPlanPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(benchRepoRoot(t), defaultSweepPlanPath)
}

func writeSweepPlanFixture(t *testing.T, root, planYAML string, profiles map[string]string) string {
	t.Helper()
	planPath := filepath.Join(root, defaultSweepPlanPath)
	writeTestFile(t, planPath, planYAML)
	for relPath, contents := range profiles {
		writeTestFile(t, filepath.Join(root, relPath), contents)
	}
	return planPath
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func minimalBenchmarkProfileYAML(id, providerType, model, baseURL, apiKeyEnv string) string {
	return fmt.Sprintf(`id: %s
provider:
  type: %s
  model: %s
  base_url: %s
  api_key_env: %s
pricing:
  input_usd_per_mtok: 0
  output_usd_per_mtok: 0
  cached_input_usd_per_mtok: 0
limits:
  max_output_tokens: 1024
  context_tokens: 8192
  rate_limit_rpm: 60
  rate_limit_tpm: 60000
sampling:
  temperature: 0
versioning:
  resolved_at: "2026-05-15"
  snapshot: "test"
`, id, providerType, model, baseURL, apiKeyEnv)
}

func TestLoadSweepPlanRejectsLaneWithoutProfile(t *testing.T) {
	root := t.TempDir()
	planPath := writeSweepPlanFixture(t, root, `dataset: terminal-bench/terminal-bench-2-1
defaults:
  reps: 1
subsets:
  - id: terminalbench-2-1-canary
    path: scripts/benchmark/task-subset-tb21-canary.yaml
    default_reps: 1
recipes:
  - id: canary
    subset: terminalbench-2-1-canary
    lanes: [fiz-vidar-ds4-mtp]
resource_groups:
  - id: rg-vidar-ds4
    provider_type: ds4
    base_url: http://192.168.2.106:1236/v1
    max_concurrency: 1
lanes:
  - id: fiz-vidar-ds4-mtp
    profile_id: vidar-ds4-mtp
    lane_type: fiz_provider_native
    comparison_groups: []
    resource_group: rg-vidar-ds4
    fizeau_env:
      FIZEAU_PROVIDER: ds4
      FIZEAU_MODEL: deepseek-v4-flash
      FIZEAU_BASE_URL: http://192.168.2.106:1236/v1
      FIZEAU_API_KEY_ENV: DS4_API_KEY
    model_family: deepseek-v4-flash
    model_id: deepseek-v4-flash
    quant_label: ds4-native-bf16-mtp
    provider_surface: vidar-ds4
`, nil)

	_, err := loadSweepPlan(planPath)
	if err == nil {
		t.Fatal("loadSweepPlan succeeded, want missing profile error")
	}
	for _, want := range []string{"fiz-vidar-ds4-mtp", "vidar-ds4-mtp", "profile_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("loadSweepPlan error %q missing %q", err, want)
		}
	}
}

func TestLoadSweepPlanRejectsLaneProfileMismatch(t *testing.T) {
	root := t.TempDir()
	planPath := writeSweepPlanFixture(t, root, `dataset: terminal-bench/terminal-bench-2-1
defaults:
  reps: 1
subsets:
  - id: terminalbench-2-1-canary
    path: scripts/benchmark/task-subset-tb21-canary.yaml
    default_reps: 1
recipes:
  - id: canary
    subset: terminalbench-2-1-canary
    lanes: [fiz-openrouter-gpt-5-4-mini]
resource_groups:
  - id: rg-openrouter
    provider_type: openrouter
    base_url: https://openrouter.ai/api/v1
    max_concurrency: 2
comparison_groups:
  - id: cg-gpt-harness-fiz
    type: approximate_same_family
    lanes: [fiz-openrouter-gpt-5-4-mini]
lanes:
  - id: fiz-openrouter-gpt-5-4-mini
    profile_id: fiz-openrouter-gpt-5-4-mini
    lane_type: fiz_provider_native
    comparison_groups: [cg-gpt-harness-fiz]
    resource_group: rg-openrouter
    fizeau_env:
      FIZEAU_PROVIDER: openrouter
      FIZEAU_MODEL: openai/gpt-5.4-mini
      FIZEAU_BASE_URL: https://openrouter.ai/api/v1
      FIZEAU_API_KEY_ENV: OPENROUTER_API_KEY
    model_family: gpt-5-mini
    model_id: openai/gpt-5.4-mini
    quant_label: cloud-hosted
    provider_surface: openrouter
`, map[string]string{
		filepath.Join("scripts", "benchmark", "profiles", "fiz-openrouter-gpt-5-4-mini.yaml"): minimalBenchmarkProfileYAML(
			"fiz-openrouter-gpt-5-4-mini",
			"openrouter",
			"openai/gpt-5.5",
			"https://openrouter.ai/api/v1",
			"OPENROUTER_API_KEY",
		),
	})

	_, err := loadSweepPlan(planPath)
	if err == nil {
		t.Fatal("loadSweepPlan succeeded, want lane/profile mismatch error")
	}
	for _, want := range []string{"fiz-openrouter-gpt-5-4-mini", "profile", "FIZEAU_MODEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("loadSweepPlan error %q missing %q", err, want)
		}
	}
}

// TestLoadSweepPlanParsesAllRecipes verifies the sweep plan YAML loads and
// contains all expected recipes (in YAML order, which is the iteration contract
// for --all-recipes and --staged-recipes).
func TestLoadSweepPlanParsesAllRecipes(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	wantRecipes := []string{"canary", "local-qwen", "timing-baseline", "or-passing", "tb21-all", "openai-cheap", "sonnet-comparison", "gpt-comparison", "medium-model-canary", "medium-model"}
	if len(plan.Recipes) != len(wantRecipes) {
		t.Fatalf("recipes = %d, want %d", len(plan.Recipes), len(wantRecipes))
	}
	for i, want := range wantRecipes {
		if got := plan.Recipes[i].ID; got != want {
			t.Errorf("recipes[%d].ID = %q, want %q", i, got, want)
		}
	}
}

// TestSweepPlanSchemaShape verifies the v2 schema invariants: subsets[] and
// recipes[] populated, no top-level Phases field on the struct, no Phases
// field on lanes. (AC-1.)
func TestSweepPlanSchemaShape(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	if len(plan.Subsets) == 0 {
		t.Fatal("plan.Subsets is empty; v2 schema requires subsets[]")
	}
	if len(plan.Recipes) == 0 {
		t.Fatal("plan.Recipes is empty; v2 schema requires recipes[]")
	}
	for _, r := range plan.Recipes {
		if r.Subset == "" {
			t.Errorf("recipe %q has empty subset reference", r.ID)
		}
		if len(r.Lanes) == 0 {
			t.Errorf("recipe %q has no lanes", r.ID)
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
		"fiz-harness-pi-gpt-5-4-mini",
		"fiz-harness-opencode-gpt-5-4-mini",
		"fiz-openrouter-claude-sonnet-4-6",
		"fiz-openrouter-gpt-5-4-mini",
		"fiz-openai-gpt-5-5",
		"fiz-openrouter-qwen3-6-27b",
		"fiz-vidar-omlx-qwen3-6-27b",
		"fiz-vidar-ds4",
		"fiz-vidar-ds4-mtp",
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

	for _, id := range []string{"fiz-harness-codex-gpt-5-4-mini", "fiz-harness-pi-gpt-5-4-mini", "fiz-harness-opencode-gpt-5-4-mini", "fiz-openrouter-gpt-5-4-mini"} {
		cgs := cgByLane[id]
		found := false
		for _, cg := range cgs {
			if cg == "cg-gpt-harness-fiz" {
				found = true
			}
		}
		if !found {
			t.Errorf("lane %s not in cg-gpt-harness-fiz", id)
		}
	}
}

// TestBuildSweepRecipeRunsAllRecipes verifies that --all-recipes expands to
// every recipe in YAML order.
func TestBuildSweepRecipeRunsAllRecipes(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	runs, err := buildSweepRecipeRuns(plan, sweepSelector{allRecipes: true})
	if err != nil {
		t.Fatalf("buildSweepRecipeRuns(--all-recipes): %v", err)
	}
	if len(runs) != len(plan.Recipes) {
		t.Fatalf("got %d runs, want %d", len(runs), len(plan.Recipes))
	}
	for i, r := range plan.Recipes {
		if runs[i].ID != r.ID {
			t.Errorf("run %d ID = %q, want %q", i, runs[i].ID, r.ID)
		}
	}
}

// TestBuildSweepRecipeRunsSingleRecipe verifies each recipe can be selected by id.
func TestBuildSweepRecipeRunsSingleRecipe(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	for _, id := range []string{"canary", "local-qwen", "tb21-all", "openai-cheap", "sonnet-comparison", "gpt-comparison", "medium-model-canary", "medium-model"} {
		runs, err := buildSweepRecipeRuns(plan, sweepSelector{recipeID: id})
		if err != nil {
			t.Errorf("buildSweepRecipeRuns(--recipe=%q): %v", id, err)
			continue
		}
		if len(runs) != 1 || runs[0].ID != id {
			t.Errorf("buildSweepRecipeRuns(--recipe=%q) = %v, want single run with that id", id, runs)
		}
	}
}

// TestBuildSweepRecipeRunsUnknownErrors verifies unknown recipe IDs error.
func TestBuildSweepRecipeRunsUnknownErrors(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	_, err = buildSweepRecipeRuns(plan, sweepSelector{recipeID: "no-such-recipe"})
	if err == nil {
		t.Fatal("expected error for unknown recipe, got nil")
	}
}

// TestSweepResolveSubsetPathFromPlan verifies subset paths resolve via the
// plan's subsets[] block (preferred) and fall back to the hardcoded map.
func TestSweepResolveSubsetPathFromPlan(t *testing.T) {
	wd := benchRepoRoot(t)
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	cases := map[string]string{
		"terminalbench-2-1-canary":          "scripts/benchmark/task-subset-tb21-canary.yaml",
		"terminalbench-2-1-full":            "scripts/benchmark/task-subset-tb21-full.yaml",
		"terminalbench-2-1-all":             "scripts/benchmark/task-subset-tb21-all.yaml",
		"terminalbench-2-1-openai-cheap":    "scripts/benchmark/task-subset-tb21-openai-cheap.yaml",
		"terminalbench-2-1-or-passing":      "scripts/benchmark/task-subset-tb21-or-passing.yaml",
		"terminalbench-2-1-timing-baseline": "scripts/benchmark/task-subset-tb21-timing-baseline.yaml",
	}
	for id, rel := range cases {
		got := sweepResolveSubsetPath(wd, plan, id)
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

	code, output := captureStdout(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--sweep-plan", filepath.Join(repoRoot, defaultSweepPlanPath),
			"--phase", "canary",
			"--dry-run",
			"--out", outDir,
		})
	})

	if code != 0 {
		t.Fatalf("cmdSweep dry-run exit = %d, want 0\noutput:\n%s", code, output)
	}

	// AC-1: print phases, lane ids, comparison_group ids, task count, reps,
	// resource groups, max parallelism, and output directory.
	required := []string{
		"Recipe: canary",
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

// TestSweepDryRunStagedRecipesContainsAllRecipeHeaders verifies --staged-recipes
// prints headers for every staged recipe (the historical --phase all gate).
func TestSweepDryRunStagedRecipesContainsAllRecipeHeaders(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	code, output := captureStdout(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--staged-recipes",
			"--dry-run",
			"--out", outDir,
		})
	})

	if code != 0 {
		t.Fatalf("cmdSweep dry-run --staged-recipes exit = %d\noutput:\n%s", code, output)
	}
	for _, recipe := range []string{"canary", "local-qwen", "sonnet-comparison", "gpt-comparison", "medium-model-canary", "medium-model"} {
		if !strings.Contains(output, "Recipe: "+recipe) {
			t.Errorf("dry-run output missing Recipe: %s", recipe)
		}
	}
}

func TestSweepDryRunFullWithLaneFilterPrintsOnlySelectedLanes(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()

	code, output := captureStdout(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--sweep-plan", filepath.Join(repoRoot, defaultSweepPlanPath),
			"--recipe", "tb21-all",
			"--lanes", "fiz-sindri-vllm-qwen3-6-27b,fiz-vidar-omlx-qwen3-6-27b",
			"--dry-run",
			"--out", outDir,
		})
	})

	if code != 0 {
		t.Fatalf("cmdSweep dry-run filtered full exit = %d\noutput:\n%s", code, output)
	}
	required := []string{
		"Recipe: tb21-all",
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

	code, output := captureStdout(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--phase", "tb21-all",
			"--lanes", "fiz-openai-gpt-5-5,fiz-openrouter-qwen3-6-27b,fiz-sindri-llamacpp-qwen3-6-27b,fiz-vidar-omlx-qwen3-6-27b",
			"--matrix-jobs-managed", "16",
			"--dry-run",
			"--out", outDir,
		})
	})

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

	recipe := plan.Recipes[0] // canary
	run := sweepRecipeRun{ID: recipe.ID, SubsetID: recipe.Subset, Reps: 3, Lanes: recipe.Lanes}
	subsetPath := sweepResolveSubsetPath(wd, plan, run.SubsetID)
	laneOutDir := filepath.Join(opts.outDir, run.ID, lane.ID)
	matrixArgs := buildSweepMatrixArgs(opts, run, lane, rg, subsetPath, laneOutDir, 3)

	meta := buildSweepLaneMeta(opts, run, lane, rg, subsetPath, laneOutDir, 3, matrixArgs)

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
		retryInvalid:    true,
		forceRerun:      false,
		perRunBudgetUSD: 5.0,
		rgByID:          sweepRGMap(plan),
		laneByID:        sweepLaneMap(plan),
	}
	lane := opts.laneByID["fiz-openrouter-claude-sonnet-4-6"]
	rg := opts.rgByID[lane.ResourceGroup]
	recipe := plan.Recipes[0]
	run := sweepRecipeRun{ID: recipe.ID, SubsetID: recipe.Subset, Reps: 3, Lanes: recipe.Lanes}
	subsetPath := sweepResolveSubsetPath(wd, plan, run.SubsetID)
	laneOutDir := filepath.Join(opts.outDir, run.ID, lane.ID)

	args := buildSweepMatrixArgs(opts, run, lane, rg, subsetPath, laneOutDir, 3)
	argStr := strings.Join(args, " ")

	if !strings.Contains(argStr, "--resume") {
		t.Error("matrix args missing --resume")
	}
	if !strings.Contains(argStr, "--retry-invalid") {
		t.Error("matrix args missing --retry-invalid")
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
	recipe := plan.Recipes[0]
	run := sweepRecipeRun{ID: recipe.ID, SubsetID: recipe.Subset, Reps: 1, Lanes: recipe.Lanes}
	subsetPath := sweepResolveSubsetPath(wd, plan, run.SubsetID)
	laneOutDir := filepath.Join(opts.outDir, run.ID, lane.ID)

	args := buildSweepMatrixArgs(opts, run, lane, rg, subsetPath, laneOutDir, 1)
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
		Subsets: []sweepSubset{
			{ID: "terminalbench-2-1-canary", Path: "scripts/benchmark/task-subset-tb21-canary.yaml", DefaultReps: 1},
		},
		Recipes: []sweepRecipe{
			{ID: "test-recipe", Subset: "terminalbench-2-1-canary", Reps: 1,
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

	for i, laneID := range plan.Recipes[0].Lanes {
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

// captureStderr swaps os.Stderr with a pipe for the duration of fn and returns
// the captured stderr contents alongside fn's exit code.
func captureStderr(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r) //nolint:errcheck
		close(done)
	}()
	code := fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	os.Stderr = old
	<-done
	if err := r.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return code, buf.String()
}

// TestSweepRecipeAliasArgs (AC-2): buildSweepRecipeRuns with --recipe canary
// produces the same shape as legacy --phase canary.
func TestSweepRecipeAliasArgs(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	runsRecipe, err := buildSweepRecipeRuns(plan, sweepSelector{
		recipeID:   "canary",
		laneFilter: "fiz-sindri-lucebox-qwen3-6-27b",
	})
	if err != nil {
		t.Fatalf("buildSweepRecipeRuns(--recipe=canary): %v", err)
	}
	if len(runsRecipe) != 1 {
		t.Fatalf("got %d runs, want 1", len(runsRecipe))
	}
	r := runsRecipe[0]
	if r.ID != "canary" {
		t.Errorf("run.ID = %q, want canary", r.ID)
	}
	if r.SubsetID != "terminalbench-2-1-canary" {
		t.Errorf("run.SubsetID = %q, want terminalbench-2-1-canary", r.SubsetID)
	}
	if len(r.Lanes) != 1 || r.Lanes[0] != "fiz-sindri-lucebox-qwen3-6-27b" {
		t.Errorf("run.Lanes = %v, want [fiz-sindri-lucebox-qwen3-6-27b]", r.Lanes)
	}
	if r.Reps != 3 {
		t.Errorf("run.Reps = %d, want 3 (canary recipe inherits subset default)", r.Reps)
	}
}

// TestSweepSubsetAdhoc (AC-3): --subset X --lanes Y produces an ad-hoc run with
// no recipe consulted; cell paths use the canonical cells/ template via --cells-root.
func TestSweepSubsetAdhoc(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	runs, err := buildSweepRecipeRuns(plan, sweepSelector{
		subsetID:   "terminalbench-2-1-all",
		laneFilter: "fiz-sindri-lucebox-qwen3-6-27b",
	})
	if err != nil {
		t.Fatalf("buildSweepRecipeRuns(--subset): %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	r := runs[0]
	if !r.IsAdhoc {
		t.Error("run.IsAdhoc = false, want true for --subset invocation")
	}
	if r.ID != "adhoc-terminalbench-2-1-all" {
		t.Errorf("run.ID = %q, want adhoc-terminalbench-2-1-all", r.ID)
	}
	if r.SubsetID != "terminalbench-2-1-all" {
		t.Errorf("run.SubsetID = %q, want terminalbench-2-1-all", r.SubsetID)
	}
	// Verify the matrix args still use the canonical --cells-root path under <OUT>/cells.
	wd := benchRepoRoot(t)
	opts := sweepRunOpts{
		plan:     plan,
		wd:       wd,
		outDir:   "/tmp/test-adhoc",
		rgByID:   sweepRGMap(plan),
		laneByID: sweepLaneMap(plan),
	}
	lane := opts.laneByID["fiz-sindri-lucebox-qwen3-6-27b"]
	rg := opts.rgByID[lane.ResourceGroup]
	subsetPath := sweepResolveSubsetPath(wd, plan, r.SubsetID)
	args := buildSweepMatrixArgs(opts, r, lane, rg, subsetPath, "/tmp/test-adhoc/"+r.ID+"/"+lane.ID, r.Reps)
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--cells-root /tmp/test-adhoc/cells") {
		t.Errorf("matrix args missing canonical --cells-root path: %s", argStr)
	}
}

// TestSweepEmptyRecipeLanesIntersectErrors (AC-4): explicit --recipe + --lanes
// with empty intersection exits 2 (input error) with a clear stderr message.
func TestSweepEmptyRecipeLanesIntersectErrors(t *testing.T) {
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	code, stderr := captureStderr(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--recipe", "sonnet-comparison",
			"--lanes", "fiz-sindri-lucebox-qwen3-6-27b",
			"--dry-run",
			"--out", outDir,
		})
	})
	if code != 2 {
		t.Errorf("cmdSweep exit = %d, want 2 (input error)", code)
	}
	want := "lane fiz-sindri-lucebox-qwen3-6-27b not in recipe sonnet-comparison"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr missing %q\nstderr:\n%s", want, stderr)
	}
}

// TestSweepPhaseDeprecationAlias (AC-5): --phase X writes a stderr deprecation
// notice and resolves to the same recipe shape as --recipe X.
func TestSweepPhaseDeprecationAlias(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	// Compare resolved shapes directly via buildSweepRecipeRuns is not enough — the
	// deprecation rewrite happens inside cmdSweep. Invoke cmdSweep with --phase and
	// confirm stderr carries the deprecation marker.
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	code, stderr := captureStderr(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--phase", "canary",
			"--lanes", "fiz-sindri-lucebox-qwen3-6-27b",
			"--dry-run",
			"--out", outDir,
		})
	})
	if code != 0 {
		t.Fatalf("cmdSweep --phase canary exit = %d, want 0\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("stderr missing 'deprecated' marker\nstderr:\n%s", stderr)
	}
	// And the resolved shape should match --recipe canary.
	runsRecipe, err := buildSweepRecipeRuns(plan, sweepSelector{recipeID: "canary", laneFilter: "fiz-sindri-lucebox-qwen3-6-27b"})
	if err != nil {
		t.Fatalf("buildSweepRecipeRuns(--recipe canary): %v", err)
	}
	if len(runsRecipe) != 1 || runsRecipe[0].ID != "canary" {
		t.Fatalf("recipe-mode resolution returned unexpected shape: %v", runsRecipe)
	}
}

// TestSweepAllRecipesWarnsAndSkipsPerRecipe (AC-6): --all-recipes --lanes Y
// runs only recipes whose lane lists overlap Y; emits one "skipping recipe"
// stderr line per non-matching recipe. lucebox is in 4 of 10 recipes today.
func TestSweepAllRecipesWarnsAndSkipsPerRecipe(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	runs, err := buildSweepRecipeRuns(plan, sweepSelector{
		allRecipes: true,
		laneFilter: "fiz-sindri-lucebox-qwen3-6-27b",
	})
	if err != nil {
		t.Fatalf("buildSweepRecipeRuns(--all-recipes): %v", err)
	}
	want := []string{"canary", "local-qwen", "or-passing", "tb21-all"}
	if len(runs) != len(want) {
		t.Fatalf("got %d runs, want %d (lucebox-containing recipes)", len(runs), len(want))
	}
	for i, w := range want {
		if runs[i].ID != w {
			t.Errorf("run[%d].ID = %q, want %q", i, runs[i].ID, w)
		}
	}
	// And cmdSweep --all-recipes --lanes lucebox should print one "skipping recipe" line per non-lucebox recipe.
	repoRoot := benchRepoRoot(t)
	outDir := t.TempDir()
	_, stderr := captureStderr(t, func() int {
		return cmdSweep([]string{
			"--work-dir", repoRoot,
			"--all-recipes",
			"--lanes", "fiz-sindri-lucebox-qwen3-6-27b",
			"--dry-run",
			"--out", outDir,
		})
	})
	skips := strings.Count(stderr, "[sweep] skipping recipe ")
	wantSkips := len(plan.Recipes) - len(want)
	if skips != wantSkips {
		t.Errorf("stderr has %d 'skipping recipe' lines, want %d\nstderr:\n%s", skips, wantSkips, stderr)
	}
}

// TestSweepMatrixJobsManagedAuto (AC-7): sweepMatrixJobs derives jobs as
// min(opts.matrixJobsManaged, recipe.max_concurrency_override (if set), rg.max_concurrency).
func TestSweepMatrixJobsManagedAuto(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	laneByID := sweepLaneMap(plan)
	rgByID := sweepRGMap(plan)
	recipeByID := sweepRecipeMap(plan)
	cases := []struct {
		recipe string
		lane   string
		cli    int
		want   int
	}{
		// tb21-all has no override; rg-openrouter-qwen36-27b max_concurrency=10 caps below cli=16.
		{"tb21-all", "fiz-openrouter-qwen3-6-27b", 16, 10},
		// local-qwen has no override; rg-sindri-lucebox-dflash max_concurrency=1 caps below all.
		{"local-qwen", "fiz-sindri-lucebox-qwen3-6-27b", 16, 1},
		// sonnet-comparison has max_concurrency_override=5; caps below cli=16 and rg=10.
		{"sonnet-comparison", "fiz-openrouter-claude-sonnet-4-6", 16, 5},
		// canary has max_concurrency_override=1; caps below all.
		{"canary", "fiz-openrouter-claude-sonnet-4-6", 16, 1},
	}
	for _, c := range cases {
		t.Run(c.recipe+"/"+c.lane, func(t *testing.T) {
			r, ok := recipeByID[c.recipe]
			if !ok {
				t.Fatalf("recipe %q not found", c.recipe)
			}
			lane, ok := laneByID[c.lane]
			if !ok {
				t.Fatalf("lane %q not found", c.lane)
			}
			rg, ok := rgByID[lane.ResourceGroup]
			if !ok {
				t.Fatalf("rg %q not found", lane.ResourceGroup)
			}
			run := sweepRecipeRun{
				ID:                     r.ID,
				MaxConcurrencyOverride: r.MaxConcurrencyOverride,
			}
			opts := sweepRunOpts{matrixJobsManaged: c.cli}
			got := sweepMatrixJobs(opts, run, rg)
			if got != c.want {
				t.Errorf("sweepMatrixJobs(%s, %s, cli=%d) = %d, want %d (recipe.override=%d, rg.max=%d)",
					c.recipe, c.lane, c.cli, got, c.want, r.MaxConcurrencyOverride, rg.MaxConcurrency)
			}
		})
	}
}

// TestSweepRecipeRepsOverridesSubsetDefault (AC-8): recipe.reps wins over the
// subset default when set; subset default applies when recipe.reps is unset.
func TestSweepRecipeRepsOverridesSubsetDefault(t *testing.T) {
	plan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("loadSweepPlan: %v", err)
	}
	cases := []struct {
		recipe string
		want   int
	}{
		{"canary", 3},              // no recipe.reps → subset default 3
		{"medium-model-canary", 1}, // recipe.reps=1 overrides same subset's default 3
	}
	for _, c := range cases {
		t.Run(c.recipe, func(t *testing.T) {
			runs, err := buildSweepRecipeRuns(plan, sweepSelector{recipeID: c.recipe})
			if err != nil {
				t.Fatalf("buildSweepRecipeRuns(%q): %v", c.recipe, err)
			}
			if len(runs) != 1 {
				t.Fatalf("got %d runs, want 1", len(runs))
			}
			if runs[0].SubsetID != "terminalbench-2-1-canary" {
				t.Fatalf("runs[0].SubsetID = %q, want terminalbench-2-1-canary (both recipes use the same subset)", runs[0].SubsetID)
			}
			if runs[0].Reps != c.want {
				t.Errorf("runs[0].Reps = %d, want %d", runs[0].Reps, c.want)
			}
		})
	}
}
