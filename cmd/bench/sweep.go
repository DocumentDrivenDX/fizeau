package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/easel/fizeau/internal/benchmark/profile"
	"gopkg.in/yaml.v3"
)

// harborTaskContainerPattern matches Harbor's per-trial TerminalBench task
// containers, named like `<task_sha>__<random>-main-1`. Docker compose adds
// the `-main-1` suffix; the random middle is per-trial.
var harborTaskContainerPattern = regexp.MustCompile(`^[0-9a-f]{32}__[a-z0-9]+-main-1$`)

// sweepPlan is the parsed terminalbench-2-1-sweep.yaml.
// Only fields consumed by the runner are decoded; free-form doc fields are ignored.
//
// Schema v2 (2026-05-14): orthogonal subsets and lanes. The historical
// `phases:` block is replaced by `subsets:` (pure task lists) + `recipes:`
// (curated CLI bundles pairing one subset with a lane list). Lane definitions
// no longer enroll in phases — the runner's matrix is (subset, lane). Recipes
// are sugar for invoking a pre-curated (subset, lanes[]) pair.
type sweepPlan struct {
	SpecID           string               `yaml:"spec-id"`
	Created          string               `yaml:"created"`
	Dataset          string               `yaml:"dataset"`
	Defaults         sweepDefaults        `yaml:"defaults"`
	Subsets          []sweepSubset        `yaml:"subsets"`
	Recipes          []sweepRecipe        `yaml:"recipes"`
	ComparisonGroups []sweepCmpGroup      `yaml:"comparison_groups"`
	ResourceGroups   []sweepResourceGroup `yaml:"resource_groups"`
	Lanes            []sweepLane          `yaml:"lanes"`
	ResumePolicy     sweepResumePolicy    `yaml:"resume_policy"`
}

type sweepDefaults struct {
	Reps   int  `yaml:"reps"`
	Resume bool `yaml:"resume"`
}

// sweepSubset is a pure task list. It carries no lane information.
type sweepSubset struct {
	ID          string `yaml:"id"`
	Path        string `yaml:"path"`
	DefaultReps int    `yaml:"default_reps"`
}

// sweepRecipe is a curated bundle pairing one subset with a lane list. Recipes
// are CLI sugar — `fiz-bench sweep --recipe <id>` expands to that subset and
// lane set. The runner's executable matrix is (subset, lane); recipes do not
// constrain it. `staged: true` declares membership in the gating sequence
// iterated by `--staged-recipes` (the historical `--phase all` behavior).
type sweepRecipe struct {
	ID                     string   `yaml:"id"`
	Staged                 bool     `yaml:"staged"`
	Description            string   `yaml:"description"`
	Subset                 string   `yaml:"subset"`
	Reps                   int      `yaml:"reps,omitempty"`
	MaxConcurrencyOverride int      `yaml:"max_concurrency_override,omitempty"`
	Lanes                  []string `yaml:"lanes"`
	ParallelPolicy         string   `yaml:"parallel_policy"`
	Preflight              []string `yaml:"preflight"`
}

// sweepRecipeRun is the internal runnable shape produced by buildSweepRecipeRuns.
// Both recipe-driven and ad-hoc `--subset X --lanes Y` invocations resolve into
// a slice of these. The ID field is the path component used for the matrix
// summary directory (`<OUT>/<ID>/<lane>/matrix.json`): for recipe runs it is
// the recipe id; for ad-hoc subset runs it is `adhoc-<subset>`.
type sweepRecipeRun struct {
	ID                     string
	SubsetID               string
	SubsetPath             string
	Reps                   int
	Lanes                  []string
	MaxConcurrencyOverride int
	Description            string
	ParallelPolicy         string
	Preflight              []string
	IsAdhoc                bool // true for `--subset X --lanes Y` invocations
}

type sweepCmpGroup struct {
	ID    string   `yaml:"id"`
	Type  string   `yaml:"type"`
	Lanes []string `yaml:"lanes"`
}

type sweepResourceGroup struct {
	ID             string         `yaml:"id"`
	Server         string         `yaml:"server,omitempty"`
	BaseURL        string         `yaml:"base_url"`
	ProviderType   string         `yaml:"provider_type"`
	MaxConcurrency int            `yaml:"max_concurrency"`
	Budget         *sweepRGBudget `yaml:"budget,omitempty"`
}

type sweepRGBudget struct {
	PerRunUSDCap   float64 `yaml:"per_run_usd_cap"`
	PerPhaseUSDCap float64 `yaml:"per_phase_usd_cap"`
	PerSweepUSDCap float64 `yaml:"per_sweep_usd_cap"`
}

type sweepLane struct {
	ID              string            `yaml:"id"`
	ProfileID       string            `yaml:"profile_id"`
	LaneType        string            `yaml:"lane_type"`
	CompGroups      []string          `yaml:"comparison_groups"`
	ResourceGroup   string            `yaml:"resource_group"`
	FizeauEnv       map[string]string `yaml:"fizeau_env"`
	ModelFamily     string            `yaml:"model_family"`
	ModelID         string            `yaml:"model_id"`
	QuantLabel      string            `yaml:"quant_label"`
	ProviderSurface string            `yaml:"provider_surface"`
	Runtime         string            `yaml:"runtime,omitempty"`
	HardwareLabel   string            `yaml:"hardware_label,omitempty"`
	Endpoint        string            `yaml:"endpoint,omitempty"`
}

type sweepResumePolicy struct {
	DefaultMode   string   `yaml:"default_mode"`
	SkipStatuses  []string `yaml:"skip_statuses"`
	RetryStatuses []string `yaml:"retry_statuses"`
	ForceRerun    bool     `yaml:"force_rerun"`
}

// sweepLaneMeta is written as sweep_lane_meta.json alongside each lane's matrix
// output to carry all fields required by AC-8 for evidence import.
type sweepLaneMeta struct {
	Dataset          string            `json:"dataset"`
	DatasetVersion   string            `json:"dataset_version"`
	SubsetID         string            `json:"subset_id"`
	SubsetPath       string            `json:"subset_path"`
	Phase            string            `json:"phase"`
	LaneID           string            `json:"lane_id"`
	ComparisonGroups []string          `json:"comparison_groups"`
	ComparisonTypes  []string          `json:"comparison_types"`
	ModelFamily      string            `json:"model_family"`
	ModelID          string            `json:"model_id"`
	QuantLabel       string            `json:"quant_label"`
	ProfileID        string            `json:"profile_id"`
	LaneType         string            `json:"lane_type"`
	ProviderType     string            `json:"provider_type,omitempty"`
	ProviderSurface  string            `json:"provider_surface"`
	Runtime          string            `json:"runtime,omitempty"`
	HardwareLabel    string            `json:"hardware_label,omitempty"`
	BaseURL          string            `json:"base_url"`
	ResourceGroup    string            `json:"resource_group"`
	Reps             int               `json:"reps"`
	FizeauEnv        map[string]string `json:"fizeau_env"`
	Command          []string          `json:"command"`
	GeneratedAt      time.Time         `json:"generated_at"`
}

// sweepLaneOutcome carries the result of a single lane run for phase summary output.
type sweepLaneOutcome struct {
	laneID string
	code   int
	meta   sweepLaneMeta
	matrix *matrixOutput
}

// sweepSummaryRow is one row in the post-phase summary table (AC-9).
type sweepSummaryRow struct {
	LaneID          string
	CompGroups      []string
	NRuns           int
	NValid          int
	NInvalid        int
	InvalidCounts   map[string]int
	NPasses         int
	WallSeconds     float64
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	CostUSD         float64
	EffCostPerValid *float64
	EffCostPerPass  *float64
}

const defaultSweepPlanPath = "scripts/benchmark/terminalbench-2-1-sweep.yaml"

// sweepSubsetPaths maps symbolic subset IDs used in the sweep plan to YAML file paths.
var sweepSubsetPaths = map[string]string{
	"terminalbench-2-1-canary":          "scripts/benchmark/task-subset-tb21-canary.yaml",
	"terminalbench-2-1-full":            "scripts/benchmark/task-subset-tb21-full.yaml",
	"terminalbench-2-1-all":             "scripts/benchmark/task-subset-tb21-all.yaml",
	"terminalbench-2-1-openai-cheap":    "scripts/benchmark/task-subset-tb21-openai-cheap.yaml",
	"terminalbench-2-1-or-passing":      "scripts/benchmark/task-subset-tb21-or-passing.yaml",
	"terminalbench-2-1-timing-baseline": "scripts/benchmark/task-subset-tb21-timing-baseline.yaml",
}

func cmdSweep(args []string) int {
	parentCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	fs := flagSet("sweep")
	sweepFile := fs.String("sweep-plan", "", "Path to sweep plan YAML (default: scripts/benchmark/terminalbench-2-1-sweep.yaml)")
	recipeID := fs.String("recipe", "", "Recipe id to run (curated subset+lanes bundle). Mutually exclusive with --subset and --recipes.")
	recipesCSV := fs.String("recipes", "", "Comma-separated recipe ids to run, in order. Mutually exclusive with --recipe and --subset.")
	subsetID := fs.String("subset", "", "Ad-hoc subset id to run; requires --lanes. Bypasses recipes; cell paths key on this subset id.")
	allRecipes := fs.Bool("all-recipes", false, "Iterate every recipe in YAML order. Mutually exclusive with --recipe/--recipes/--subset.")
	stagedRecipes := fs.Bool("staged-recipes", false, "Iterate every recipe with staged: true, in YAML order (the historical --phase all gate sequence).")
	phaseID := fs.String("phase", "", "DEPRECATED: alias of --recipe (or --staged-recipes when value=all). Prints a deprecation warning to stderr.")
	laneFilter := fs.String("lanes", "", "Comma-separated lane IDs to filter within selected recipe(s). For explicit --recipe X, empty intersection with X.lanes is an error; for --all-recipes / --recipes / --staged-recipes, empty intersection per recipe is warned and skipped.")
	reps := fs.Int("reps", 0, "Repetition count override (default: recipe.reps, falling back to subset.default_reps).")
	dryRun := fs.Bool("dry-run", false, "Print plan without launching Harbor or any matrix run")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	out := fs.String("out", "", "Output directory (default: bench/results/sweep-<timestamp> under work-dir)")
	resume := fs.Bool("resume", true, "Skip terminal cells (default: true per sweep plan defaults)")
	forceRerun := fs.Bool("force-rerun", false, "Rerun even terminal cells")
	retryInvalid := fs.Bool("retry-invalid", false, "Rerun cells with non-empty invalid_class while resuming")
	tasksDir := fs.String("tasks-dir", "", "Path to TB-2.1 tasks directory; enables Harbor-graded runs")
	budgetUSD := fs.Float64("budget-usd", 0, "Total sweep budget in USD (0 = no cap)")
	perRunBudgetUSD := fs.Float64("per-run-budget-usd", 0, "Per-run budget cap in USD (0 = no cap)")
	matrixJobsManaged := fs.Int("matrix-jobs-managed", 1, "Concurrent cells per managed-provider lane (default 1; increase subject to rate caps)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	wd := resolveWorkDir(*workDir)

	planPath := *sweepFile
	if planPath == "" {
		planPath = filepath.Join(wd, defaultSweepPlanPath)
	} else if !filepath.IsAbs(planPath) {
		planPath = filepath.Join(wd, planPath)
	}

	plan, err := loadSweepPlan(planPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s sweep: load plan %s: %v\n", benchCommandName(), planPath, err)
		return 1
	}

	// Translate deprecated --phase to its --recipe / --staged-recipes equivalent.
	if *phaseID != "" {
		if *recipeID != "" || *recipesCSV != "" || *subsetID != "" || *allRecipes || *stagedRecipes {
			fmt.Fprintf(os.Stderr, "%s sweep: --phase cannot be combined with --recipe/--recipes/--subset/--all-recipes/--staged-recipes\n", benchCommandName())
			return 2
		}
		if *phaseID == "all" {
			fmt.Fprintf(os.Stderr, "%s sweep: --phase=all is deprecated; using --staged-recipes\n", benchCommandName())
			*stagedRecipes = true
		} else {
			fmt.Fprintf(os.Stderr, "%s sweep: --phase=%s is deprecated; using --recipe=%s\n", benchCommandName(), *phaseID, *phaseID)
			*recipeID = *phaseID
		}
	}

	runs, err := buildSweepRecipeRuns(plan, sweepSelector{
		recipeID:      *recipeID,
		recipesCSV:    *recipesCSV,
		subsetID:      *subsetID,
		allRecipes:    *allRecipes,
		stagedRecipes: *stagedRecipes,
		laneFilter:    *laneFilter,
		repsOverride:  *reps,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s sweep: %v\n", benchCommandName(), err)
		return 2
	}
	if len(runs) == 0 {
		fmt.Fprintf(os.Stderr, "%s sweep: no recipe/subset selected\n", benchCommandName())
		return 2
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(wd, "bench/results", "sweep-"+time.Now().UTC().Format("20060102T150405Z"))
	} else if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(wd, outDir)
	}

	opts := sweepRunOpts{
		ctx:               parentCtx,
		plan:              plan,
		wd:                wd,
		outDir:            outDir,
		resume:            *resume,
		forceRerun:        *forceRerun,
		retryInvalid:      *retryInvalid,
		tasksDir:          *tasksDir,
		budgetUSD:         *budgetUSD,
		perRunBudgetUSD:   *perRunBudgetUSD,
		matrixJobsManaged: *matrixJobsManaged,
		rgByID:            sweepRGMap(plan),
		laneByID:          sweepLaneMap(plan),
		cgByLane:          sweepCGByLane(plan),
		cgTypeByID:        sweepCGTypeMap(plan),
	}

	for _, run := range runs {
		if *dryRun {
			if code := printSweepDryRun(opts, run); code != 0 {
				return code
			}
		} else {
			if code := runSweepPhase(opts, run); code != 0 {
				pruneStaleHarborTaskContainers(time.Hour)
				return code
			}
		}
	}
	if !*dryRun {
		pruneStaleHarborTaskContainers(time.Hour)
	}
	return 0
}

// pruneStaleHarborTaskContainers stops and removes Harbor TerminalBench task
// containers older than minAge. Containers are named `<sha>__<rand>-main-1`;
// after a clean Harbor exit they are deleted by `--delete`. Anything left
// behind older than the trial timeout window (35min) is a leak — typically
// from a SIGKILL'd parent or an upstream Harbor crash. minAge is the safety
// margin so we never touch a concurrent sweep's in-flight containers.
//
// Best-effort: errors print but never fail the caller. Returns the count of
// containers removed.
func pruneStaleHarborTaskContainers(minAge time.Duration) int {
	out, err := exec.Command("docker", "ps", "-a",
		"--format", "{{.Names}}\t{{.CreatedAt}}").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[sweep] cleanup: docker ps failed: %v\n", err)
		return 0
	}
	cutoff := time.Now().Add(-minAge)
	var stale []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if !harborTaskContainerPattern.MatchString(name) {
			continue
		}
		// docker `CreatedAt` format: "2026-05-08 16:40:13 +0000 UTC"
		created, err := time.Parse("2006-01-02 15:04:05 -0700 MST", strings.TrimSpace(parts[1]))
		if err != nil || created.After(cutoff) {
			continue
		}
		stale = append(stale, name)
	}
	if len(stale) == 0 {
		return 0
	}
	fmt.Printf("[sweep] cleanup: pruning %d stale Harbor task container(s) older than %s\n", len(stale), minAge)
	args := append([]string{"rm", "-f"}, stale...)
	if err := exec.Command("docker", args...).Run(); err != nil { // #nosec G204 -- docker is fixed binary; args are container names from docker ps output
		fmt.Fprintf(os.Stderr, "[sweep] cleanup: docker rm failed: %v\n", err)
	}
	return len(stale)
}

type sweepRunOpts struct {
	ctx               context.Context
	plan              *sweepPlan
	wd                string
	outDir            string
	resume            bool
	forceRerun        bool
	retryInvalid      bool
	tasksDir          string
	budgetUSD         float64
	perRunBudgetUSD   float64
	matrixJobsManaged int
	rgByID            map[string]*sweepResourceGroup
	laneByID          map[string]*sweepLane
	cgByLane          map[string][]string // lane id → comparison group ids
	cgTypeByID        map[string]string   // cg id → comparison type
}

func printSweepDryRun(opts sweepRunOpts, run sweepRecipeRun) int {
	reps := run.Reps
	if reps == 0 {
		reps = opts.plan.Defaults.Reps
	}
	subsetPath := sweepResolveSubsetPath(opts.wd, opts.plan, run.SubsetID)
	taskCount := 0
	if s, err := loadTermbenchSubset(subsetPath); err == nil {
		taskCount = len(s.Tasks)
	}

	label := "Recipe"
	if run.IsAdhoc {
		label = "Ad-hoc Subset"
	}
	fmt.Printf("=== %s: %s ===\n", label, run.ID)
	fmt.Printf("  Dataset:       %s\n", opts.plan.Dataset)
	fmt.Printf("  Subset ID:     %s\n", run.SubsetID)
	fmt.Printf("  Subset Path:   %s\n", subsetPath)
	fmt.Printf("  Task Count:    %d\n", taskCount)
	fmt.Printf("  Reps:          %d\n", reps)
	fmt.Printf("  Total Cells:   %d\n", taskCount*reps*len(run.Lanes))
	fmt.Printf("  Output Dir:    %s\n", filepath.Join(opts.outDir, run.ID))
	fmt.Printf("  Resume:        %v\n", opts.resume)
	fmt.Printf("  Force Rerun:   %v\n", opts.forceRerun)
	fmt.Printf("  Retry Invalid: %v\n", opts.retryInvalid)
	fmt.Println()

	for _, laneID := range run.Lanes {
		lane, ok := opts.laneByID[laneID]
		if !ok {
			fmt.Fprintf(os.Stderr, "%s sweep: dry-run: lane %q not found in sweep plan\n", benchCommandName(), laneID)
			return 1
		}
		rg, ok := opts.rgByID[lane.ResourceGroup]
		if !ok {
			fmt.Fprintf(os.Stderr, "%s sweep: dry-run: resource group %q not found for lane %s\n",
				benchCommandName(), lane.ResourceGroup, laneID)
			return 1
		}
		cgs := opts.cgByLane[laneID]
		laneOutDir := filepath.Join(opts.outDir, run.ID, laneID)
		matrixArgs := buildSweepMatrixArgs(opts, run, lane, rg, subsetPath, laneOutDir, reps)
		if jobs := sweepMatrixJobs(opts, run, rg); jobs > 1 {
			matrixArgs = append(matrixArgs, "--jobs", fmt.Sprintf("%d", jobs))
		}
		cmd := append([]string{benchCommandName(), "matrix"}, matrixArgs...)

		fmt.Printf("  Lane: %s\n", laneID)
		fmt.Printf("    Profile:             %s\n", lane.ProfileID)
		fmt.Printf("    Lane Type:           %s\n", lane.LaneType)
		if len(cgs) > 0 {
			fmt.Printf("    Comparison Groups:   %s\n", strings.Join(cgs, ", "))
		} else {
			fmt.Printf("    Comparison Groups:   (none)\n")
		}
		fmt.Printf("    Resource Group:      %s (provider=%s, max_concurrency=%d)\n",
			rg.ID, rg.ProviderType, rg.MaxConcurrency)
		fmt.Printf("    Model Family:        %s\n", lane.ModelFamily)
		fmt.Printf("    Model ID:            %s\n", lane.ModelID)
		fmt.Printf("    Quant/Config Label:  %s\n", lane.QuantLabel)
		fmt.Printf("    Task Count:          %d\n", taskCount)
		fmt.Printf("    Reps:                %d\n", reps)
		fmt.Printf("    Total Cells:         %d\n", taskCount*reps)
		fmt.Printf("    Output Dir:          %s\n", laneOutDir)
		fmt.Printf("    Command:             %s\n", strings.Join(cmd, " "))
		if rg.Budget != nil {
			fmt.Printf("    Budget/run cap:      $%.2f  phase cap: $%.2f  sweep cap: $%.2f\n",
				rg.Budget.PerRunUSDCap, rg.Budget.PerPhaseUSDCap, rg.Budget.PerSweepUSDCap)
		}
		fmt.Println()
	}
	return 0
}

func runSweepPhase(opts sweepRunOpts, run sweepRecipeRun) int {
	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	reps := run.Reps
	if reps == 0 {
		reps = opts.plan.Defaults.Reps
	}
	subsetPath := sweepResolveSubsetPath(opts.wd, opts.plan, run.SubsetID)

	// Per-resource-group semaphores enforce max_concurrency across concurrent lane goroutines.
	rgSems := make(map[string]chan struct{}, len(opts.plan.ResourceGroups))
	for _, rg := range opts.plan.ResourceGroups {
		cap := rg.MaxConcurrency
		if cap < 1 {
			cap = 1
		}
		rgSems[rg.ID] = make(chan struct{}, cap)
	}

	outcomes := make([]sweepLaneOutcome, len(run.Lanes))
	var wg sync.WaitGroup

	for i, laneID := range run.Lanes {
		if ctx.Err() != nil {
			outcomes[i] = sweepLaneOutcome{laneID: laneID, code: 130}
			continue
		}
		lane, ok := opts.laneByID[laneID]
		if !ok {
			fmt.Fprintf(os.Stderr, "%s sweep: lane %q not found in plan\n", benchCommandName(), laneID)
			return 1
		}
		rg, ok := opts.rgByID[lane.ResourceGroup]
		if !ok {
			fmt.Fprintf(os.Stderr, "%s sweep: resource group %q not found for lane %s\n",
				benchCommandName(), lane.ResourceGroup, laneID)
			return 1
		}
		sem, ok := rgSems[rg.ID]
		if !ok {
			sem = make(chan struct{}, 1)
			rgSems[rg.ID] = sem
		}

		wg.Add(1)
		go func(i int, lane *sweepLane, rg *sweepResourceGroup, sem chan struct{}) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				outcomes[i] = sweepLaneOutcome{laneID: lane.ID, code: 130}
				return
			}
			defer func() { <-sem }()

			laneOutDir := filepath.Join(opts.outDir, run.ID, lane.ID)
			jobs := sweepMatrixJobs(opts, run, rg)
			matrixArgs := buildSweepMatrixArgs(opts, run, lane, rg, subsetPath, laneOutDir, reps)
			if jobs > 1 {
				matrixArgs = append(matrixArgs, "--jobs", fmt.Sprintf("%d", jobs))
			}

			meta := buildSweepLaneMeta(opts, run, lane, rg, subsetPath, laneOutDir, reps, matrixArgs)
			if err := os.MkdirAll(laneOutDir, 0o750); err != nil {
				fmt.Fprintf(os.Stderr, "%s sweep: mkdir %s: %v\n", benchCommandName(), laneOutDir, err)
				outcomes[i] = sweepLaneOutcome{laneID: lane.ID, code: 1, meta: meta}
				return
			}
			if err := writeJSONAtomic(filepath.Join(laneOutDir, "sweep_lane_meta.json"), meta); err != nil {
				fmt.Fprintf(os.Stderr, "%s sweep: write lane meta %s: %v\n", benchCommandName(), lane.ID, err)
				outcomes[i] = sweepLaneOutcome{laneID: lane.ID, code: 1, meta: meta}
				return
			}

			fmt.Printf("[sweep] recipe=%s lane=%s rg=%s (max_concurrency=%d) starting\n",
				run.ID, lane.ID, rg.ID, rg.MaxConcurrency)
			code := cmdMatrixWithContext(ctx, matrixArgs)
			fmt.Printf("[sweep] recipe=%s lane=%s exit=%d\n", run.ID, lane.ID, code)

			var mout *matrixOutput
			matrixPath := filepath.Join(laneOutDir, "matrix.json")
			if raw, err := os.ReadFile(matrixPath); err == nil { // #nosec G304 -- runner-owned output path
				var mo matrixOutput
				if json.Unmarshal(raw, &mo) == nil {
					mout = &mo
				}
			}
			outcomes[i] = sweepLaneOutcome{laneID: lane.ID, code: code, meta: meta, matrix: mout}
		}(i, lane, rg, sem)
	}
	wg.Wait()

	printSweepPhaseSummary(run.ID, outcomes)

	for _, o := range outcomes {
		if o.code != 0 {
			return o.code
		}
	}
	return 0
}

// sweepMatrixJobs computes the per-lane --jobs value passed to fiz-bench matrix.
// jobs = min(cli --matrix-jobs-managed, recipe.max_concurrency_override (if set),
// rg.max_concurrency). Floor of 1.
func sweepMatrixJobs(opts sweepRunOpts, run sweepRecipeRun, rg *sweepResourceGroup) int {
	jobs := opts.matrixJobsManaged
	if jobs < 1 {
		jobs = 1
	}
	if run.MaxConcurrencyOverride > 0 && run.MaxConcurrencyOverride < jobs {
		jobs = run.MaxConcurrencyOverride
	}
	if rg.MaxConcurrency > 0 && rg.MaxConcurrency < jobs {
		jobs = rg.MaxConcurrency
	}
	if jobs < 1 {
		jobs = 1
	}
	return jobs
}

func printSweepPhaseSummary(phaseID string, outcomes []sweepLaneOutcome) {
	fmt.Printf("\n=== Phase Summary: %s ===\n", phaseID)
	hdr := fmt.Sprintf("%-42s %5s %5s %7s %10s %10s %10s %12s %14s %14s",
		"Lane", "Pass", "Fail", "Invalid", "WallSec", "InToks", "OutToks", "Cost($)", "EffCost/Valid", "EffCost/Pass")
	fmt.Println(hdr)
	fmt.Println(strings.Repeat("-", len(hdr)))

	for _, o := range outcomes {
		row := summarizeSweepLane(o.laneID, o.matrix)
		effValid := "-"
		effPass := "-"
		if row.EffCostPerValid != nil {
			effValid = fmt.Sprintf("$%.4f", *row.EffCostPerValid)
		}
		if row.EffCostPerPass != nil {
			effPass = fmt.Sprintf("$%.4f", *row.EffCostPerPass)
		}
		fmt.Printf("%-42s %5d %5d %7d %10.1f %10d %10d %12.4f %14s %14s\n",
			truncStr(o.laneID, 42),
			row.NPasses,
			row.NValid-row.NPasses,
			row.NInvalid,
			row.WallSeconds,
			row.InputTokens,
			row.OutputTokens,
			row.CostUSD,
			effValid,
			effPass,
		)
	}
	fmt.Println()
}

func summarizeSweepLane(laneID string, mout *matrixOutput) sweepSummaryRow {
	row := sweepSummaryRow{LaneID: laneID}
	if mout == nil {
		return row
	}
	for _, run := range mout.Runs {
		row.NRuns++
		row.WallSeconds += floatVal(run.WallSeconds)
		row.InputTokens += intValue(run.InputTokens)
		row.OutputTokens += intValue(run.OutputTokens)
		row.CachedTokens += intValue(run.CachedInputTokens)
		row.CostUSD += run.CostUSD
		if ic := classifyMatrixInvalid(run); ic != "" {
			row.NInvalid++
			if row.InvalidCounts == nil {
				row.InvalidCounts = map[string]int{}
			}
			row.InvalidCounts[ic]++
		} else {
			row.NValid++
			if run.Reward != nil && *run.Reward == 1 {
				row.NPasses++
			}
		}
	}
	if row.NValid > 0 {
		v := row.CostUSD / float64(row.NValid)
		row.EffCostPerValid = &v
	}
	if row.NPasses > 0 {
		v := row.CostUSD / float64(row.NPasses)
		row.EffCostPerPass = &v
	}
	return row
}

func floatVal(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// buildSweepMatrixArgs constructs the argument slice for cmdMatrix for a given lane.
func buildSweepMatrixArgs(opts sweepRunOpts, run sweepRecipeRun, lane *sweepLane, rg *sweepResourceGroup, subsetPath, laneOutDir string, reps int) []string {
	args := []string{
		"--work-dir", opts.wd,
		"--subset", subsetPath,
		"--profiles", lane.ProfileID,
		"--harnesses", "fiz",
		"--reps", fmt.Sprintf("%d", reps),
		"--out", laneOutDir,
		"--cells-root", filepath.Join(opts.outDir, "cells"),
	}
	if opts.resume {
		args = append(args, "--resume")
	}
	if opts.forceRerun {
		args = append(args, "--force-rerun")
	}
	if opts.retryInvalid {
		args = append(args, "--retry-invalid")
	}
	if opts.tasksDir != "" {
		args = append(args, "--tasks-dir", opts.tasksDir)
	}
	for _, key := range sortedMapKeys(lane.FizeauEnv) {
		args = append(args, "--env", key+"="+lane.FizeauEnv[key])
	}
	if opts.budgetUSD > 0 {
		args = append(args, "--budget-usd", fmt.Sprintf("%g", opts.budgetUSD))
	}
	// Apply resource-group per-run budget cap when present and no stricter flag is set.
	if rg.Budget != nil && rg.Budget.PerRunUSDCap > 0 && opts.perRunBudgetUSD == 0 {
		args = append(args, "--per-run-budget-usd", fmt.Sprintf("%g", rg.Budget.PerRunUSDCap))
	} else if opts.perRunBudgetUSD > 0 {
		args = append(args, "--per-run-budget-usd", fmt.Sprintf("%g", opts.perRunBudgetUSD))
	}
	return args
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func buildSweepLaneMeta(opts sweepRunOpts, run sweepRecipeRun, lane *sweepLane, rg *sweepResourceGroup, subsetPath, laneOutDir string, reps int, matrixArgs []string) sweepLaneMeta {
	cgs := opts.cgByLane[lane.ID]
	var cgTypes []string
	for _, cgID := range cgs {
		if t, ok := opts.cgTypeByID[cgID]; ok {
			cgTypes = append(cgTypes, t)
		}
	}

	cmd := append([]string{benchCommandName(), "matrix"}, matrixArgs...)

	// Redact runtime API key values from fizeau_env; keep env var name references.
	safeEnv := make(map[string]string, len(lane.FizeauEnv))
	for k, v := range lane.FizeauEnv {
		upper := strings.ToUpper(k)
		if strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_SECRET") {
			safeEnv[k] = "<redacted>"
		} else {
			safeEnv[k] = v
		}
	}

	return sweepLaneMeta{
		Dataset:          opts.plan.Dataset,
		DatasetVersion:   "2.1",
		SubsetID:         run.SubsetID,
		SubsetPath:       subsetPath,
		Phase:            run.ID,
		LaneID:           lane.ID,
		ComparisonGroups: cgs,
		ComparisonTypes:  cgTypes,
		ModelFamily:      lane.ModelFamily,
		ModelID:          lane.ModelID,
		QuantLabel:       lane.QuantLabel,
		ProfileID:        lane.ProfileID,
		LaneType:         lane.LaneType,
		ProviderType:     rg.ProviderType,
		ProviderSurface:  lane.ProviderSurface,
		Runtime:          lane.Runtime,
		HardwareLabel:    lane.HardwareLabel,
		BaseURL:          rg.BaseURL,
		ResourceGroup:    rg.ID,
		Reps:             reps,
		FizeauEnv:        safeEnv,
		Command:          cmd,
		GeneratedAt:      time.Now().UTC(),
	}
}

// sweepResolveSubsetPath maps a symbolic subset ID to a YAML file path under
// scripts/benchmark/. Plan subsets[] entries take precedence; falls back to
// the historical hardcoded map, then to treating the ID as a literal path.
func sweepResolveSubsetPath(wd string, plan *sweepPlan, subsetID string) string {
	if plan != nil {
		for _, s := range plan.Subsets {
			if s.ID == subsetID && s.Path != "" {
				if filepath.IsAbs(s.Path) {
					return s.Path
				}
				return filepath.Join(wd, s.Path)
			}
		}
	}
	if rel, ok := sweepSubsetPaths[subsetID]; ok {
		return filepath.Join(wd, rel)
	}
	if filepath.IsAbs(subsetID) {
		return subsetID
	}
	return filepath.Join(wd, subsetID)
}

func sweepSubsetMap(plan *sweepPlan) map[string]*sweepSubset {
	m := make(map[string]*sweepSubset, len(plan.Subsets))
	for i := range plan.Subsets {
		s := &plan.Subsets[i]
		m[s.ID] = s
	}
	return m
}

func sweepRecipeMap(plan *sweepPlan) map[string]*sweepRecipe {
	m := make(map[string]*sweepRecipe, len(plan.Recipes))
	for i := range plan.Recipes {
		r := &plan.Recipes[i]
		m[r.ID] = r
	}
	return m
}

func loadSweepPlan(path string) (*sweepPlan, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-controlled flag
	if err != nil {
		return nil, err
	}
	var plan sweepPlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse sweep plan: %w", err)
	}
	if len(plan.Subsets) == 0 {
		return nil, fmt.Errorf("sweep plan has no subsets")
	}
	if len(plan.Recipes) == 0 {
		return nil, fmt.Errorf("sweep plan has no recipes")
	}
	if len(plan.Lanes) == 0 {
		return nil, fmt.Errorf("sweep plan has no lanes")
	}
	// Validate every recipe references a known subset, and every recipe lane is a known lane.
	subsetByID := sweepSubsetMap(&plan)
	laneByID := sweepLaneMap(&plan)
	rgByID := sweepRGMap(&plan)
	cgByID := sweepCGMap(&plan)
	profilesDir := filepath.Join(filepath.Dir(path), "profiles")
	loadedProfiles := make(map[string]*profile.Profile)
	for i := range plan.Lanes {
		lane := &plan.Lanes[i]
		if strings.TrimSpace(lane.ProfileID) == "" {
			return nil, fmt.Errorf("lane %q profile %q missing profile_id", lane.ID, lane.ProfileID)
		}
		if _, ok := rgByID[lane.ResourceGroup]; !ok {
			return nil, fmt.Errorf("lane %q profile %q missing resource_group %q", lane.ID, lane.ProfileID, lane.ResourceGroup)
		}
		prof, err := loadSweepLaneProfile(profilesDir, loadedProfiles, lane)
		if err != nil {
			return nil, err
		}
		if err := validateSweepLaneProfile(lane, prof); err != nil {
			return nil, err
		}
		for _, groupID := range lane.CompGroups {
			if _, ok := cgByID[groupID]; !ok {
				return nil, fmt.Errorf("lane %q profile %q comparison_groups references unknown group %q", lane.ID, lane.ProfileID, groupID)
			}
		}
	}
	for _, cg := range plan.ComparisonGroups {
		for _, laneID := range cg.Lanes {
			if _, ok := laneByID[laneID]; !ok {
				return nil, fmt.Errorf("comparison_group %q references unknown lane %q", cg.ID, laneID)
			}
		}
	}
	for _, r := range plan.Recipes {
		if _, ok := subsetByID[r.Subset]; !ok {
			return nil, fmt.Errorf("recipe %q references unknown subset %q", r.ID, r.Subset)
		}
		for _, laneID := range r.Lanes {
			if _, ok := laneByID[laneID]; !ok {
				return nil, fmt.Errorf("recipe %q references unknown lane %q", r.ID, laneID)
			}
		}
	}
	return &plan, nil
}

func loadSweepLaneProfile(profilesDir string, loadedProfiles map[string]*profile.Profile, lane *sweepLane) (*profile.Profile, error) {
	if prof, ok := loadedProfiles[lane.ProfileID]; ok {
		return prof, nil
	}
	profilePath := filepath.Join(profilesDir, lane.ProfileID+".yaml")
	prof, err := profile.Load(profilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return nil, fmt.Errorf("lane %q profile %q missing profile file %q (field profile_id)", lane.ID, lane.ProfileID, profilePath)
		}
		return nil, fmt.Errorf("lane %q profile %q failed to load %q: %w", lane.ID, lane.ProfileID, profilePath, err)
	}
	loadedProfiles[lane.ProfileID] = prof
	return prof, nil
}

func validateSweepLaneProfile(lane *sweepLane, prof *profile.Profile) error {
	if prof == nil {
		return fmt.Errorf("lane %q profile %q did not load", lane.ID, lane.ProfileID)
	}
	if prof.ID != lane.ProfileID {
		return fmt.Errorf("lane %q profile %q mismatched profile.id: profile file declares %q", lane.ID, lane.ProfileID, prof.ID)
	}
	if err := validateSweepLaneEnvMatch(lane, "FIZEAU_PROVIDER", string(prof.Provider.Type), "provider.type"); err != nil {
		return err
	}
	if err := validateSweepLaneEnvMatch(lane, "FIZEAU_MODEL", prof.Provider.Model, "provider.model"); err != nil {
		return err
	}
	if err := validateSweepLaneEnvMatch(lane, "FIZEAU_BASE_URL", prof.Provider.BaseURL, "provider.base_url"); err != nil {
		return err
	}
	if err := validateSweepLaneEnvMatch(lane, "FIZEAU_API_KEY_ENV", prof.Provider.APIKeyEnv, "provider.api_key_env"); err != nil {
		return err
	}
	return nil
}

func validateSweepLaneEnvMatch(lane *sweepLane, envKey, want, profileField string) error {
	got := strings.TrimSpace(lane.FizeauEnv[envKey])
	if got == "" || strings.TrimSpace(want) == "" {
		return nil
	}
	if got != want {
		return fmt.Errorf("lane %q profile %q mismatched %s: lane has %q, profile %s has %q", lane.ID, lane.ProfileID, envKey, got, profileField, want)
	}
	return nil
}

// sweepSelector captures the CLI selection state: which recipes/subsets/lanes
// to run. buildSweepRecipeRuns resolves this into concrete []sweepRecipeRun.
type sweepSelector struct {
	recipeID      string
	recipesCSV    string
	subsetID      string
	allRecipes    bool
	stagedRecipes bool
	laneFilter    string
	repsOverride  int
}

// buildSweepRecipeRuns turns CLI selection into a list of runnable bundles.
//
// Empty-intersection semantics:
//   - Explicit `--recipe X --lanes Y` with no overlap → error (exit 1).
//   - Multi-recipe mode (`--all-recipes`, `--staged-recipes`, `--recipes a,b`)
//     with `--lanes Y` → per-recipe warn+skip; succeed as long as at least
//     one recipe contributed lanes. If every recipe was skipped, the caller
//     gets an empty slice; cmdSweep treats that as "no recipe/subset selected".
//
// `--subset X --lanes Y` is the ad-hoc path: no recipe is consulted, the
// returned run has IsAdhoc=true and ID=`adhoc-<X>`.
func buildSweepRecipeRuns(plan *sweepPlan, sel sweepSelector) ([]sweepRecipeRun, error) {
	// Mode mutual exclusion.
	modes := 0
	if sel.recipeID != "" {
		modes++
	}
	if sel.recipesCSV != "" {
		modes++
	}
	if sel.subsetID != "" {
		modes++
	}
	if sel.allRecipes {
		modes++
	}
	if sel.stagedRecipes {
		modes++
	}
	if modes == 0 {
		return nil, fmt.Errorf("one of --recipe, --recipes, --subset, --all-recipes, --staged-recipes is required")
	}
	if modes > 1 {
		return nil, fmt.Errorf("--recipe/--recipes/--subset/--all-recipes/--staged-recipes are mutually exclusive")
	}

	laneFilter, err := parseSweepLaneFilter(plan, sel.laneFilter)
	if err != nil {
		return nil, err
	}

	// Ad-hoc subset path.
	if sel.subsetID != "" {
		subsetByID := sweepSubsetMap(plan)
		sub, ok := subsetByID[sel.subsetID]
		if !ok {
			var ids []string
			for _, s := range plan.Subsets {
				ids = append(ids, s.ID)
			}
			return nil, fmt.Errorf("unknown subset %q; valid: %s", sel.subsetID, strings.Join(ids, ", "))
		}
		if len(laneFilter) == 0 {
			return nil, fmt.Errorf("--subset requires --lanes")
		}
		reps := sel.repsOverride
		if reps <= 0 {
			reps = sub.DefaultReps
		}
		if reps <= 0 {
			reps = plan.Defaults.Reps
		}
		return []sweepRecipeRun{{
			ID:         "adhoc-" + sub.ID,
			SubsetID:   sub.ID,
			SubsetPath: sub.Path, // resolved at call site via sweepResolveSubsetPath if needed
			Reps:       reps,
			Lanes:      laneFilter,
			IsAdhoc:    true,
		}}, nil
	}

	// Recipe path(s).
	recipeByID := sweepRecipeMap(plan)
	var selectedRecipes []*sweepRecipe
	multi := false
	switch {
	case sel.allRecipes:
		multi = true
		for i := range plan.Recipes {
			selectedRecipes = append(selectedRecipes, &plan.Recipes[i])
		}
	case sel.stagedRecipes:
		multi = true
		for i := range plan.Recipes {
			if plan.Recipes[i].Staged {
				selectedRecipes = append(selectedRecipes, &plan.Recipes[i])
			}
		}
		if len(selectedRecipes) == 0 {
			return nil, fmt.Errorf("--staged-recipes selected but no recipe has staged: true")
		}
	case sel.recipesCSV != "":
		multi = true
		for _, raw := range strings.Split(sel.recipesCSV, ",") {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			r, ok := recipeByID[id]
			if !ok {
				var ids []string
				for _, r := range plan.Recipes {
					ids = append(ids, r.ID)
				}
				return nil, fmt.Errorf("unknown recipe %q; valid: %s", id, strings.Join(ids, ", "))
			}
			selectedRecipes = append(selectedRecipes, r)
		}
	default: // sel.recipeID != ""
		r, ok := recipeByID[sel.recipeID]
		if !ok {
			var ids []string
			for _, r := range plan.Recipes {
				ids = append(ids, r.ID)
			}
			return nil, fmt.Errorf("unknown recipe %q; valid: %s", sel.recipeID, strings.Join(ids, ", "))
		}
		selectedRecipes = []*sweepRecipe{r}
	}

	subsetByID := sweepSubsetMap(plan)
	var runs []sweepRecipeRun
	for _, r := range selectedRecipes {
		laneSet := map[string]bool{}
		for _, laneID := range r.Lanes {
			laneSet[laneID] = true
		}
		var lanes []string
		if len(laneFilter) == 0 {
			lanes = append(lanes, r.Lanes...)
		} else {
			for _, laneID := range laneFilter {
				if laneSet[laneID] {
					lanes = append(lanes, laneID)
				}
			}
		}
		if len(lanes) == 0 {
			if multi {
				fmt.Fprintf(os.Stderr, "[sweep] skipping recipe %s: no overlap between --lanes and recipe lanes\n", r.ID)
				continue
			}
			return nil, fmt.Errorf("lane %s not in recipe %s; recipe lanes: %s",
				sel.laneFilter, r.ID, strings.Join(r.Lanes, ","))
		}
		sub := subsetByID[r.Subset] // existence validated by loadSweepPlan
		reps := sel.repsOverride
		if reps <= 0 {
			reps = r.Reps
		}
		if reps <= 0 && sub != nil {
			reps = sub.DefaultReps
		}
		if reps <= 0 {
			reps = plan.Defaults.Reps
		}
		runs = append(runs, sweepRecipeRun{
			ID:                     r.ID,
			SubsetID:               r.Subset,
			SubsetPath:             sub.Path,
			Reps:                   reps,
			Lanes:                  lanes,
			MaxConcurrencyOverride: r.MaxConcurrencyOverride,
			Description:            r.Description,
			ParallelPolicy:         r.ParallelPolicy,
			Preflight:              r.Preflight,
		})
	}
	return runs, nil
}

// parseSweepLaneFilter validates a comma-separated lane id list and returns
// the unique ids in input order. Empty filter returns nil with no error.
func parseSweepLaneFilter(plan *sweepPlan, raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	known := sweepLaneMap(plan)
	seen := map[string]bool{}
	var ordered []string
	for _, p := range strings.Split(raw, ",") {
		id := strings.TrimSpace(p)
		if id == "" {
			continue
		}
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("lane %q not found in sweep plan", id)
		}
		if !seen[id] {
			seen[id] = true
			ordered = append(ordered, id)
		}
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("--lanes did not include any lane IDs")
	}
	return ordered, nil
}

func sweepRGMap(plan *sweepPlan) map[string]*sweepResourceGroup {
	m := make(map[string]*sweepResourceGroup, len(plan.ResourceGroups))
	for i := range plan.ResourceGroups {
		rg := &plan.ResourceGroups[i]
		m[rg.ID] = rg
	}
	return m
}

func sweepCGMap(plan *sweepPlan) map[string]*sweepCmpGroup {
	m := make(map[string]*sweepCmpGroup, len(plan.ComparisonGroups))
	for i := range plan.ComparisonGroups {
		cg := &plan.ComparisonGroups[i]
		m[cg.ID] = cg
	}
	return m
}

func sweepLaneMap(plan *sweepPlan) map[string]*sweepLane {
	m := make(map[string]*sweepLane, len(plan.Lanes))
	for i := range plan.Lanes {
		l := &plan.Lanes[i]
		m[l.ID] = l
	}
	return m
}

func sweepCGByLane(plan *sweepPlan) map[string][]string {
	m := make(map[string][]string)
	for _, cg := range plan.ComparisonGroups {
		for _, laneID := range cg.Lanes {
			m[laneID] = append(m[laneID], cg.ID)
		}
	}
	return m
}

func sweepCGTypeMap(plan *sweepPlan) map[string]string {
	m := make(map[string]string, len(plan.ComparisonGroups))
	for _, cg := range plan.ComparisonGroups {
		m[cg.ID] = cg.Type
	}
	return m
}

// sweepMatrixSummary reads all matrix.json files under a sweep output directory
// and returns aggregated summary rows per lane for model-selection calculus.
func sweepMatrixSummary(sweepOutDir string) ([]sweepSummaryRow, error) {
	entries, err := os.ReadDir(sweepOutDir)
	if err != nil {
		return nil, err
	}
	var rows []sweepSummaryRow
	for _, phaseEntry := range entries {
		if !phaseEntry.IsDir() {
			continue
		}
		phaseDir := filepath.Join(sweepOutDir, phaseEntry.Name())
		laneEntries, err := os.ReadDir(phaseDir)
		if err != nil {
			continue
		}
		for _, laneEntry := range laneEntries {
			if !laneEntry.IsDir() {
				continue
			}
			matrixPath := filepath.Join(phaseDir, laneEntry.Name(), "matrix.json")
			raw, err := os.ReadFile(matrixPath) // #nosec G304 -- runner-owned output path
			if err != nil {
				continue
			}
			var mout matrixOutput
			if err := json.Unmarshal(raw, &mout); err != nil {
				continue
			}
			row := summarizeSweepLane(laneEntry.Name(), &mout)
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].LaneID < rows[j].LaneID })
	return rows, nil
}
