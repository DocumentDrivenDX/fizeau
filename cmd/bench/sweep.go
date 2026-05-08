package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// sweepPlan is the parsed terminalbench-2-1-sweep.yaml.
// Only fields consumed by the runner are decoded; free-form doc fields are ignored.
type sweepPlan struct {
	SpecID           string               `yaml:"spec-id"`
	Created          string               `yaml:"created"`
	Dataset          string               `yaml:"dataset"`
	Defaults         sweepDefaults        `yaml:"defaults"`
	Phases           []sweepPhase         `yaml:"phases"`
	ComparisonGroups []sweepCmpGroup      `yaml:"comparison_groups"`
	ResourceGroups   []sweepResourceGroup `yaml:"resource_groups"`
	Lanes            []sweepLane          `yaml:"lanes"`
	ResumePolicy     sweepResumePolicy    `yaml:"resume_policy"`
}

type sweepDefaults struct {
	Reps   int  `yaml:"reps"`
	Resume bool `yaml:"resume"`
}

type sweepPhase struct {
	ID             string   `yaml:"id"`
	Description    string   `yaml:"description"`
	Reps           int      `yaml:"reps"`
	Subset         string   `yaml:"subset"`
	Lanes          []string `yaml:"lanes"`
	ParallelPolicy string   `yaml:"parallel_policy"`
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
	Phases          []string          `yaml:"phases"`
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
	"terminalbench-2-1-canary":       "scripts/benchmark/task-subset-tb21-canary.yaml",
	"terminalbench-2-1-full":         "scripts/benchmark/task-subset-tb21-full.yaml",
	"terminalbench-2-1-all":          "scripts/benchmark/task-subset-tb21-all.yaml",
	"terminalbench-2-1-openai-cheap": "scripts/benchmark/task-subset-tb21-openai-cheap.yaml",
}

func cmdSweep(args []string) int {
	fs := flagSet("sweep")
	sweepFile := fs.String("sweep-plan", "", "Path to sweep plan YAML (default: scripts/benchmark/terminalbench-2-1-sweep.yaml)")
	phaseID := fs.String("phase", "all", "Phase to run: canary, openai-cheap, local-qwen, sonnet-comparison, gpt-comparison, tb21-all, or all")
	laneFilter := fs.String("lanes", "", "Comma-separated lane IDs to run from the selected phase(s)")
	dryRun := fs.Bool("dry-run", false, "Print plan without launching Harbor or any matrix run")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	out := fs.String("out", "", "Output directory (default: benchmark-results/sweep-<timestamp> under work-dir)")
	resume := fs.Bool("resume", true, "Skip terminal cells (default: true per sweep plan defaults)")
	forceRerun := fs.Bool("force-rerun", false, "Rerun even terminal cells")
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

	phases, err := selectSweepPhases(plan, *phaseID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s sweep: %v\n", benchCommandName(), err)
		return 2
	}
	phases, err = filterSweepPhasesByLanes(plan, phases, *laneFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s sweep: %v\n", benchCommandName(), err)
		return 2
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(wd, "benchmark-results", "sweep-"+time.Now().UTC().Format("20060102T150405Z"))
	} else if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(wd, outDir)
	}

	opts := sweepRunOpts{
		plan:              plan,
		wd:                wd,
		outDir:            outDir,
		resume:            *resume,
		forceRerun:        *forceRerun,
		tasksDir:          *tasksDir,
		budgetUSD:         *budgetUSD,
		perRunBudgetUSD:   *perRunBudgetUSD,
		matrixJobsManaged: *matrixJobsManaged,
		rgByID:            sweepRGMap(plan),
		laneByID:          sweepLaneMap(plan),
		cgByLane:          sweepCGByLane(plan),
		cgTypeByID:        sweepCGTypeMap(plan),
	}

	for _, phase := range phases {
		if *dryRun {
			if code := printSweepDryRun(opts, phase); code != 0 {
				return code
			}
		} else {
			if code := runSweepPhase(opts, phase); code != 0 {
				return code
			}
		}
	}
	return 0
}

type sweepRunOpts struct {
	plan              *sweepPlan
	wd                string
	outDir            string
	resume            bool
	forceRerun        bool
	tasksDir          string
	budgetUSD         float64
	perRunBudgetUSD   float64
	matrixJobsManaged int
	rgByID            map[string]*sweepResourceGroup
	laneByID          map[string]*sweepLane
	cgByLane          map[string][]string // lane id → comparison group ids
	cgTypeByID        map[string]string   // cg id → comparison type
}

func printSweepDryRun(opts sweepRunOpts, phase sweepPhase) int {
	reps := phase.Reps
	if reps == 0 {
		reps = opts.plan.Defaults.Reps
	}
	subsetPath := sweepResolveSubsetPath(opts.wd, phase.Subset)
	taskCount := 0
	if s, err := loadTermbenchSubset(subsetPath); err == nil {
		taskCount = len(s.Tasks)
	}

	fmt.Printf("=== Phase: %s ===\n", phase.ID)
	fmt.Printf("  Dataset:       %s\n", opts.plan.Dataset)
	fmt.Printf("  Subset ID:     %s\n", phase.Subset)
	fmt.Printf("  Subset Path:   %s\n", subsetPath)
	fmt.Printf("  Task Count:    %d\n", taskCount)
	fmt.Printf("  Reps:          %d\n", reps)
	fmt.Printf("  Total Cells:   %d\n", taskCount*reps*len(phase.Lanes))
	fmt.Printf("  Output Dir:    %s\n", filepath.Join(opts.outDir, phase.ID))
	fmt.Printf("  Resume:        %v\n", opts.resume)
	fmt.Printf("  Force Rerun:   %v\n", opts.forceRerun)
	fmt.Println()

	for _, laneID := range phase.Lanes {
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
		laneOutDir := filepath.Join(opts.outDir, phase.ID, laneID)
		matrixArgs := buildSweepMatrixArgs(opts, phase, lane, rg, subsetPath, laneOutDir, reps)
		if jobs := sweepMatrixJobs(opts, rg); jobs > 1 {
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

func runSweepPhase(opts sweepRunOpts, phase sweepPhase) int {
	reps := phase.Reps
	if reps == 0 {
		reps = opts.plan.Defaults.Reps
	}
	subsetPath := sweepResolveSubsetPath(opts.wd, phase.Subset)

	// Per-resource-group semaphores enforce max_concurrency across concurrent lane goroutines.
	rgSems := make(map[string]chan struct{}, len(opts.plan.ResourceGroups))
	for _, rg := range opts.plan.ResourceGroups {
		cap := rg.MaxConcurrency
		if cap < 1 {
			cap = 1
		}
		rgSems[rg.ID] = make(chan struct{}, cap)
	}

	outcomes := make([]sweepLaneOutcome, len(phase.Lanes))
	var wg sync.WaitGroup

	for i, laneID := range phase.Lanes {
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
			sem <- struct{}{}
			defer func() { <-sem }()

			laneOutDir := filepath.Join(opts.outDir, phase.ID, lane.ID)
			jobs := sweepMatrixJobs(opts, rg)
			matrixArgs := buildSweepMatrixArgs(opts, phase, lane, rg, subsetPath, laneOutDir, reps)
			if jobs > 1 {
				matrixArgs = append(matrixArgs, "--jobs", fmt.Sprintf("%d", jobs))
			}

			meta := buildSweepLaneMeta(opts, phase, lane, rg, subsetPath, laneOutDir, reps, matrixArgs)
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

			fmt.Printf("[sweep] phase=%s lane=%s rg=%s (max_concurrency=%d) starting\n",
				phase.ID, lane.ID, rg.ID, rg.MaxConcurrency)
			code := cmdMatrix(matrixArgs)
			fmt.Printf("[sweep] phase=%s lane=%s exit=%d\n", phase.ID, lane.ID, code)

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

	printSweepPhaseSummary(phase.ID, outcomes)

	for _, o := range outcomes {
		if o.code != 0 {
			return o.code
		}
	}
	return 0
}

func sweepMatrixJobs(opts sweepRunOpts, rg *sweepResourceGroup) int {
	jobs := 1
	if rg.MaxConcurrency > 1 && opts.matrixJobsManaged > 1 {
		jobs = opts.matrixJobsManaged
		if jobs > rg.MaxConcurrency {
			jobs = rg.MaxConcurrency
		}
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
func buildSweepMatrixArgs(opts sweepRunOpts, phase sweepPhase, lane *sweepLane, rg *sweepResourceGroup, subsetPath, laneOutDir string, reps int) []string {
	args := []string{
		"--work-dir", opts.wd,
		"--subset", subsetPath,
		"--profiles", lane.ProfileID,
		"--harnesses", "fiz",
		"--reps", fmt.Sprintf("%d", reps),
		"--out", laneOutDir,
	}
	if opts.resume {
		args = append(args, "--resume")
	}
	if opts.forceRerun {
		args = append(args, "--force-rerun")
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

func buildSweepLaneMeta(opts sweepRunOpts, phase sweepPhase, lane *sweepLane, rg *sweepResourceGroup, subsetPath, laneOutDir string, reps int, matrixArgs []string) sweepLaneMeta {
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
		SubsetID:         phase.Subset,
		SubsetPath:       subsetPath,
		Phase:            phase.ID,
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
// scripts/benchmark/, falling back to treating the ID as a direct path.
func sweepResolveSubsetPath(wd, subsetID string) string {
	if rel, ok := sweepSubsetPaths[subsetID]; ok {
		return filepath.Join(wd, rel)
	}
	if filepath.IsAbs(subsetID) {
		return subsetID
	}
	return filepath.Join(wd, subsetID)
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
	if len(plan.Phases) == 0 {
		return nil, fmt.Errorf("sweep plan has no phases")
	}
	if len(plan.Lanes) == 0 {
		return nil, fmt.Errorf("sweep plan has no lanes")
	}
	return &plan, nil
}

func selectSweepPhases(plan *sweepPlan, phaseID string) ([]sweepPhase, error) {
	if phaseID == "all" {
		return plan.Phases, nil
	}
	for _, p := range plan.Phases {
		if p.ID == phaseID {
			return []sweepPhase{p}, nil
		}
	}
	var ids []string
	for _, p := range plan.Phases {
		ids = append(ids, p.ID)
	}
	return nil, fmt.Errorf("unknown phase %q; valid: %s", phaseID, strings.Join(ids, ", "))
}

func filterSweepPhasesByLanes(plan *sweepPlan, phases []sweepPhase, laneFilter string) ([]sweepPhase, error) {
	laneFilter = strings.TrimSpace(laneFilter)
	if laneFilter == "" {
		return phases, nil
	}

	known := sweepLaneMap(plan)
	wanted := map[string]bool{}
	var ordered []string
	for _, raw := range strings.Split(laneFilter, ",") {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("lane %q not found in sweep plan", id)
		}
		if !wanted[id] {
			wanted[id] = true
			ordered = append(ordered, id)
		}
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("--lanes did not include any lane IDs")
	}

	filtered := make([]sweepPhase, 0, len(phases))
	for _, phase := range phases {
		next := phase
		next.Lanes = nil
		phaseLaneSet := map[string]bool{}
		for _, laneID := range phase.Lanes {
			phaseLaneSet[laneID] = true
		}
		for _, laneID := range ordered {
			if phaseLaneSet[laneID] {
				next.Lanes = append(next.Lanes, laneID)
			}
		}
		if len(next.Lanes) > 0 {
			filtered = append(filtered, next)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("none of --lanes %q are present in selected phase(s)", laneFilter)
	}
	return filtered, nil
}

func sweepRGMap(plan *sweepPlan) map[string]*sweepResourceGroup {
	m := make(map[string]*sweepResourceGroup, len(plan.ResourceGroups))
	for i := range plan.ResourceGroups {
		rg := &plan.ResourceGroups[i]
		m[rg.ID] = rg
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
