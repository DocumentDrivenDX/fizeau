package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/benchmark/evidence"
)

const fhiFormulaRelativePath = "scripts/benchmark/fhi-formula-v1.json"

type fhiFormulaConfig struct {
	Version                string                `json:"version"`
	Name                   string                `json:"name"`
	EvidenceWindow         string                `json:"evidence_window"`
	Scale                  float64               `json:"scale"`
	BenchmarkWeights       []fhiFormulaBenchmark `json:"benchmark_weights"`
	DeltaCoverageAxes      []string              `json:"delta_coverage_axes"`
	RankRequiredBenchmarks []string              `json:"rank_required_benchmarks"`
	InvalidRunHandling     fhiInvalidRunHandling `json:"invalid_run_handling"`
	CoverageNotes          []string              `json:"coverage_notes"`
	ConfidenceNotes        []string              `json:"confidence_notes"`
	ExclusionRules         []string              `json:"exclusion_rules"`
}

type fhiFormulaBenchmark struct {
	Benchmark       string  `json:"benchmark"`
	Weight          float64 `json:"weight"`
	Primary         bool    `json:"primary"`
	ScoreMetric     string  `json:"score_metric"`
	RequiredForRank bool    `json:"required_for_rank"`
	Role            string  `json:"role"`
}

type fhiInvalidRunHandling struct {
	Policy string `json:"policy"`
	Note   string `json:"note"`
}

type fhiSubjectClaim struct {
	key      string
	score    float64
	view     map[string]any
	covered  []string
	benchMap map[string]*benchmarkAggregate
}

func cmdFHI(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s fhi <subcommand>\n\nSubcommands:\n  delta   Compare two benchmark records on a single benchmark axis\n  rank    Rank subjects by cross-benchmark FHI\n", benchCommandName())
		return 2
	}

	switch args[0] {
	case "delta":
		return cmdFHIDelta(args[1:])
	case "rank":
		return cmdFHIRank(args[1:])
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stderr, "Usage: %s fhi delta --ledger <ledger.jsonl> --left <record_id> --right <record_id>\n       %s fhi rank --ledger <ledger.jsonl>\n", benchCommandName(), benchCommandName())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s fhi: unknown subcommand %q\n", benchCommandName(), args[0])
		return 2
	}
}

func cmdFHIDelta(args []string) int {
	fs := flagSet("fhi delta")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	ledgerPath := fs.String("ledger", "", "Benchmark evidence ledger JSONL")
	leftID := fs.String("left", "", "Left record_id")
	rightID := fs.String("right", "", "Right record_id")
	format := fs.String("format", "json", "Output format: json|text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*ledgerPath) == "" || strings.TrimSpace(*leftID) == "" || strings.TrimSpace(*rightID) == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s fhi delta --ledger <ledger.jsonl> --left <record_id> --right <record_id>\n", benchCommandName())
		return 2
	}

	wd := resolveWorkDir(*workDir)
	formula, err := loadFHIFormula(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s fhi delta: %v\n", benchCommandName(), err)
		return 1
	}
	records, err := loadFHIRecords(wd, *ledgerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s fhi delta: %v\n", benchCommandName(), err)
		return 1
	}
	claim, err := buildFHIDeltaClaim(formula, records, *leftID, *rightID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s fhi delta: %v\n", benchCommandName(), err)
		return 1
	}
	return emitFHIClaim(claim, *format, claimStatus(claim), os.Stdout)
}

func cmdFHIRank(args []string) int {
	fs := flagSet("fhi rank")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	ledgerPath := fs.String("ledger", "", "Benchmark evidence ledger JSONL")
	format := fs.String("format", "json", "Output format: json|text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*ledgerPath) == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s fhi rank --ledger <ledger.jsonl>\n", benchCommandName())
		return 2
	}

	wd := resolveWorkDir(*workDir)
	formula, err := loadFHIFormula(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s fhi rank: %v\n", benchCommandName(), err)
		return 1
	}
	records, err := loadFHIRecords(wd, *ledgerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s fhi rank: %v\n", benchCommandName(), err)
		return 1
	}
	claim, err := buildFHIRankClaim(formula, records)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s fhi rank: %v\n", benchCommandName(), err)
		return 1
	}
	return emitFHIClaim(claim, *format, claimStatus(claim), os.Stdout)
}

func loadFHIFormula(repoRoot string) (*fhiFormulaConfig, error) {
	path := filepath.Join(repoRoot, fhiFormulaRelativePath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read FHI formula %s: %w", path, err)
	}
	var cfg fhiFormulaConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse FHI formula %s: %w", path, err)
	}
	if cfg.Version == "" {
		return nil, fmt.Errorf("FHI formula %s is missing version", path)
	}
	return &cfg, nil
}

func loadFHIRecords(repoRoot, ledgerPath string) ([]map[string]any, error) {
	validator, err := evidence.NewValidator(repoRoot)
	if err != nil {
		return nil, err
	}
	return validator.ValidateFile(ledgerPath)
}

func buildFHIDeltaClaim(formula *fhiFormulaConfig, records []map[string]any, leftID, rightID string) (map[string]any, error) {
	left := findRecordByID(records, leftID)
	right := findRecordByID(records, rightID)
	if left == nil {
		return nil, fmt.Errorf("left record_id %q not found", leftID)
	}
	if right == nil {
		return nil, fmt.Errorf("right record_id %q not found", rightID)
	}

	if reason := compareDeltaAxes(formula, left, right); reason != "" {
		return map[string]any{
			"claim_type": "benchmark_delta",
			"status":     "refused",
			"reason":     reason,
			"formula":    formulaSnapshot(formula),
			"ledger":     summarizeFHILedger(records),
		}, nil
	}

	leftScore, err := normalizedScore(left)
	if err != nil {
		return nil, err
	}
	rightScore, err := normalizedScore(right)
	if err != nil {
		return nil, err
	}
	delta := roundToOne(leftScore - rightScore)

	claim := map[string]any{
		"claim_type": "benchmark_delta",
		"status":     "ok",
		"formula":    formulaSnapshot(formula),
		"benchmark":  benchmarkSnapshot(left),
		"delta":      delta,
		"left":       recordClaimView(left),
		"right":      recordClaimView(right),
		"ledger":     summarizeFHILedger(records),
	}
	claim["claim_text"] = buildDeltaClaimText(left, right, delta)
	return claim, nil
}

func buildFHIRankClaim(formula *fhiFormulaConfig, records []map[string]any) (map[string]any, error) {
	ledger := summarizeFHILedger(records)
	subjects := groupFHIRecordsBySubject(formula, records)
	requiredBenchmarks := requiredRankBenchmarks(formula)

	if len(requiredBenchmarks) == 0 {
		return nil, fmt.Errorf("FHI formula %s has no rank-required benchmarks", formula.Version)
	}

	var claims []fhiSubjectClaim
	for key, group := range subjects {
		view, covered, score, reason := claimForSubject(group, requiredBenchmarks, formula)
		if reason != "" {
			return map[string]any{
				"claim_type": "fhi_rank",
				"status":     "refused",
				"reason":     reason,
				"formula":    formulaSnapshot(formula),
				"ledger":     ledger,
			}, nil
		}
		claims = append(claims, fhiSubjectClaim{
			key:      key,
			score:    score,
			view:     view,
			covered:  covered,
			benchMap: group,
		})
	}

	if len(claims) < 2 {
		return map[string]any{
			"claim_type": "fhi_rank",
			"status":     "refused",
			"reason":     "need at least two comparable subjects",
			"formula":    formulaSnapshot(formula),
			"ledger":     ledger,
		}, nil
	}

	sort.Slice(claims, func(i, j int) bool {
		if claims[i].score == claims[j].score {
			return claims[i].key < claims[j].key
		}
		return claims[i].score > claims[j].score
	})

	rankings := make([]map[string]any, 0, len(claims))
	for idx, claim := range claims {
		ranking := map[string]any{
			"rank":               idx + 1,
			"subject_key":        claim.key,
			"fhi":                roundToOne(claim.score),
			"covered_benchmarks": claim.covered,
			"subject":            claim.view,
			"benchmarks":         benchmarkClaimViews(claim.benchMap),
		}
		rankings = append(rankings, ranking)
	}

	out := map[string]any{
		"claim_type": "fhi_rank",
		"status":     "ok",
		"formula":    formulaSnapshot(formula),
		"ledger":     ledger,
		"rankings":   rankings,
	}
	if len(claims) >= 2 {
		out["delta"] = roundToOne(claims[0].score - claims[1].score)
	}
	out["claim_text"] = buildRankClaimText(claims)
	return out, nil
}

func emitFHIClaim(claim map[string]any, format string, status string, out *os.File) int {
	if strings.EqualFold(format, "text") {
		if text, _ := claim["claim_text"].(string); text != "" {
			fmt.Fprintln(out, text)
		} else {
			fmt.Fprintln(out, status)
		}
		return claimExitCode(status)
	}
	raw, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: marshal claim: %v\n", benchCommandName(), err)
		return 1
	}
	fmt.Fprintln(out, string(raw))
	return claimExitCode(status)
}

func claimExitCode(status string) int {
	if strings.EqualFold(status, "refused") {
		return 1
	}
	return 0
}

func claimStatus(claim map[string]any) string {
	if claim == nil {
		return ""
	}
	status, _ := claim["status"].(string)
	return status
}

func findRecordByID(records []map[string]any, recordID string) map[string]any {
	for _, record := range records {
		if stringField(record, "record_id") == recordID {
			return record
		}
	}
	return nil
}

func compareDeltaAxes(formula *fhiFormulaConfig, left, right map[string]any) string {
	leftBenchmark := benchmarkSnapshot(left)
	rightBenchmark := benchmarkSnapshot(right)
	axes := formula.DeltaCoverageAxes
	if len(axes) == 0 {
		axes = []string{
			"benchmark.name",
			"benchmark.version",
			"benchmark.dataset_commit",
			"benchmark.subset_id",
			"benchmark.subset_version",
			"benchmark.scorer",
			"benchmark.scorer_version",
			"scope.denominator_rule",
			"coverage.formula_version",
			"coverage.evidence_window",
		}
	}
	for _, axis := range axes {
		lv, lok := fhiLookupPath(left, axis)
		rv, rok := fhiLookupPath(right, axis)
		if !lok || !rok {
			return fmt.Sprintf("missing comparison axis %s", axis)
		}
		if fmt.Sprint(lv) != fmt.Sprint(rv) {
			return fmt.Sprintf("comparison axis %s differs: %v vs %v", axis, lv, rv)
		}
	}
	if fhiStringPath(left, "subject.model_raw") != fhiStringPath(right, "subject.model_raw") {
		return fmt.Sprintf("model differs: %s vs %s", fhiStringPath(left, "subject.model_raw"), fhiStringPath(right, "subject.model_raw"))
	}
	if fhiStringPath(left, "subject.provider") != fhiStringPath(right, "subject.provider") {
		return fmt.Sprintf("provider differs: %s vs %s", fhiStringPath(left, "subject.provider"), fhiStringPath(right, "subject.provider"))
	}
	if leftBenchmark["name"] == nil || rightBenchmark["name"] == nil {
		return "missing benchmark metadata"
	}
	return ""
}

func normalizedScore(record map[string]any) (float64, error) {
	score := mapValue(record, "score")
	if score == nil {
		return 0, fmt.Errorf("record %s missing score", stringField(record, "record_id"))
	}
	value, ok := fhiFloatValue(score["value"])
	if !ok {
		return 0, fmt.Errorf("record %s has invalid score.value", stringField(record, "record_id"))
	}
	if value <= 1 {
		return value * 100, nil
	}
	return value, nil
}

func roundToOne(v float64) float64 {
	return math.Round(v*10) / 10
}

func formulaSnapshot(formula *fhiFormulaConfig) map[string]any {
	if formula == nil {
		return nil
	}
	benchmarks := make([]map[string]any, 0, len(formula.BenchmarkWeights))
	for _, bench := range formula.BenchmarkWeights {
		benchmarks = append(benchmarks, map[string]any{
			"benchmark":         bench.Benchmark,
			"weight":            bench.Weight,
			"primary":           bench.Primary,
			"score_metric":      bench.ScoreMetric,
			"required_for_rank": bench.RequiredForRank,
			"role":              bench.Role,
		})
	}
	return map[string]any{
		"version":                  formula.Version,
		"name":                     formula.Name,
		"evidence_window":          formula.EvidenceWindow,
		"scale":                    formula.Scale,
		"benchmark_weights":        benchmarks,
		"delta_coverage_axes":      append([]string(nil), formula.DeltaCoverageAxes...),
		"rank_required_benchmarks": append([]string(nil), formula.RankRequiredBenchmarks...),
		"invalid_run_handling":     map[string]any{"policy": formula.InvalidRunHandling.Policy, "note": formula.InvalidRunHandling.Note},
		"coverage_notes":           append([]string(nil), formula.CoverageNotes...),
		"confidence_notes":         append([]string(nil), formula.ConfidenceNotes...),
		"exclusion_rules":          append([]string(nil), formula.ExclusionRules...),
	}
}

func benchmarkSnapshot(record map[string]any) map[string]any {
	bench := mapValue(record, "benchmark")
	if bench == nil {
		return nil
	}
	return map[string]any{
		"name":             stringField(bench, "name"),
		"version":          stringField(bench, "version"),
		"dataset":          stringField(bench, "dataset"),
		"dataset_commit":   stringField(bench, "dataset_commit"),
		"subset_id":        stringField(bench, "subset_id"),
		"subset_version":   stringField(bench, "subset_version"),
		"scorer":           stringField(bench, "scorer"),
		"scorer_version":   stringField(bench, "scorer_version"),
		"higher_is_better": fhiBoolValue(bench["higher_is_better"]),
	}
}

func recordClaimView(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	return map[string]any{
		"record_id":     stringField(record, "record_id"),
		"captured_at":   stringField(record, "captured_at"),
		"source":        mapValue(record, "source"),
		"benchmark":     benchmarkSnapshot(record),
		"subject":       mapValue(record, "subject"),
		"scope":         mapValue(record, "scope"),
		"score":         mapValue(record, "score"),
		"coverage":      mapValue(record, "coverage"),
		"denominator":   mapValue(record, "denominator"),
		"runtime":       mapValue(record, "runtime"),
		"provenance":    mapValue(record, "provenance"),
		"invalid_class": stringOrEmpty(record, "invalid_class"),
	}
}

func buildDeltaClaimText(left, right map[string]any, delta float64) string {
	leftHarness := fhiStringPath(left, "subject.harness")
	rightHarness := fhiStringPath(right, "subject.harness")
	leftModel := firstNonEmptyString(fhiStringPath(left, "subject.model_raw"), fhiStringPath(left, "subject.model"))
	rightModel := firstNonEmptyString(fhiStringPath(right, "subject.model_raw"), fhiStringPath(right, "subject.model"))
	if leftModel == rightModel && fhiStringPath(left, "subject.provider") == fhiStringPath(right, "subject.provider") {
		return fmt.Sprintf("%s with %s scores %.1f on %s, %.1f points %s %s on the same subset.",
			leftHarness, leftModel, benchmarkScoreDisplay(left), benchmarkName(left), abs(delta), deltaDirection(delta), rightHarness)
	}
	return fmt.Sprintf("%s scores %.1f versus %s at %.1f on %s; delta %.1f.",
		leftHarness, benchmarkScoreDisplay(left), rightHarness, benchmarkScoreDisplay(right), benchmarkName(left), delta)
}

func benchmarkScoreDisplay(record map[string]any) float64 {
	score, err := normalizedScore(record)
	if err != nil {
		return 0
	}
	return score
}

func benchmarkName(record map[string]any) string {
	return fhiStringPath(record, "benchmark.name")
}

func deltaDirection(delta float64) string {
	if delta >= 0 {
		return "ahead of"
	}
	return "behind"
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

type benchmarkAggregate struct {
	Benchmark    map[string]any
	Score        float64
	Records      []map[string]any
	RepIDs       []int
	SourceHashes []string
}

func groupFHIRecordsBySubject(formula *fhiFormulaConfig, records []map[string]any) map[string]map[string]*benchmarkAggregate {
	subjects := map[string]map[string]*benchmarkAggregate{}
	required := make(map[string]struct{}, len(formula.BenchmarkWeights))
	for _, bench := range formula.BenchmarkWeights {
		if bench.Weight <= 0 {
			continue
		}
		required[bench.Benchmark] = struct{}{}
	}

	for _, record := range records {
		if !fhiBoolValue(mapValue(record, "denominator")["included"]) {
			continue
		}
		benchName := fhiStringPath(record, "benchmark.name")
		if _, ok := required[benchName]; !ok {
			continue
		}
		subjectKey := subjectKey(record)
		if subjectKey == "" {
			continue
		}
		subject := subjects[subjectKey]
		if subject == nil {
			subject = map[string]*benchmarkAggregate{}
			subjects[subjectKey] = subject
		}
		agg := subject[benchName]
		if agg == nil {
			agg = &benchmarkAggregate{
				Benchmark: benchmarkSnapshot(record),
			}
			subject[benchName] = agg
		}
		score, err := normalizedScore(record)
		if err != nil {
			continue
		}
		agg.Score += score
		agg.Records = append(agg.Records, record)
		if rep, ok := fhiIntValue(mapValue(record, "scope")["rep"]); ok {
			agg.RepIDs = append(agg.RepIDs, rep)
		}
		if hash := fhiStringPath(record, "source.artifact_sha256"); hash != "" {
			agg.SourceHashes = append(agg.SourceHashes, hash)
		}
	}

	for _, subject := range subjects {
		for _, agg := range subject {
			if len(agg.Records) > 0 {
				agg.Score = agg.Score / float64(len(agg.Records))
			}
		}
	}
	return subjects
}

func requiredRankBenchmarks(formula *fhiFormulaConfig) []string {
	if len(formula.RankRequiredBenchmarks) > 0 {
		return append([]string(nil), formula.RankRequiredBenchmarks...)
	}
	out := make([]string, 0, len(formula.BenchmarkWeights))
	for _, bench := range formula.BenchmarkWeights {
		if bench.RequiredForRank || bench.Weight > 0 {
			out = append(out, bench.Benchmark)
		}
	}
	return out
}

func claimForSubject(group map[string]*benchmarkAggregate, requiredBenchmarks []string, formula *fhiFormulaConfig) (map[string]any, []string, float64, string) {
	covered := make([]string, 0, len(requiredBenchmarks))
	benchmarks := map[string]any{}
	weights := weightMap(formula)
	var total float64
	for _, benchName := range requiredBenchmarks {
		agg, ok := group[benchName]
		if !ok || agg == nil || len(agg.Records) == 0 {
			return nil, nil, 0, fmt.Sprintf("missing required benchmark coverage for %s", benchName)
		}
		covered = append(covered, benchName)
		benchmarks[benchName] = benchmarkAggregateView(agg)
		total += agg.Score * weights[benchName]
	}
	subject := selectRepresentativeSubject(group)
	if subject == nil {
		return nil, nil, 0, "missing subject details"
	}
	view := map[string]any{
		"record_id":  stringField(subject, "record_id"),
		"subject":    mapValue(subject, "subject"),
		"runtime":    mapValue(subject, "runtime"),
		"provenance": mapValue(subject, "provenance"),
		"coverage":   mapValue(subject, "coverage"),
		"benchmarks": benchmarks,
	}
	return view, covered, total, ""
}

func selectRepresentativeSubject(group map[string]*benchmarkAggregate) map[string]any {
	keys := make([]string, 0, len(group))
	for key := range group {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var fallback map[string]any
	for _, key := range keys {
		agg := group[key]
		if agg != nil && len(agg.Records) > 0 {
			for _, record := range agg.Records {
				if mapValue(record, "runtime") != nil {
					return record
				}
			}
			if fallback == nil {
				fallback = agg.Records[0]
			}
		}
	}
	return fallback
}

func benchmarkAggregateView(agg *benchmarkAggregate) map[string]any {
	if agg == nil {
		return nil
	}
	reps := append([]int(nil), agg.RepIDs...)
	sort.Ints(reps)
	return map[string]any{
		"benchmark":     agg.Benchmark,
		"score":         roundToOne(agg.Score),
		"record_count":  len(agg.Records),
		"reps":          reps,
		"source_hashes": uniqueStrings(agg.SourceHashes),
	}
}

func benchmarkClaimViews(group map[string]*benchmarkAggregate) map[string]any {
	out := make(map[string]any, len(group))
	keys := make([]string, 0, len(group))
	for key := range group {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = benchmarkAggregateView(group[key])
	}
	return out
}

func weightMap(formula *fhiFormulaConfig) map[string]float64 {
	out := make(map[string]float64, len(formula.BenchmarkWeights))
	var total float64
	for _, bench := range formula.BenchmarkWeights {
		if bench.Weight <= 0 {
			continue
		}
		out[bench.Benchmark] = bench.Weight
		total += bench.Weight
	}
	if total > 0 {
		for k, v := range out {
			out[k] = v / total
		}
	}
	return out
}

func summarizeFHILedger(records []map[string]any) map[string]any {
	invalidByClass := map[string]any{}
	invalidCount := 0
	denominatorPolicies := map[string]any{}
	for _, record := range records {
		denom := mapValue(record, "denominator")
		if policy := stringField(denom, "policy"); policy != "" {
			denominatorPolicies[policy] = intValueAny(denominatorPolicies[policy]) + 1
		}
		if !fhiBoolValue(denom["included"]) {
			invalidCount++
			class := stringOrEmpty(record, "invalid_class")
			if class == "" {
				class = stringField(denom, "policy")
			}
			if class == "" {
				class = "invalid"
			}
			invalidByClass[class] = intValueAny(invalidByClass[class]) + 1
		}
	}
	return map[string]any{
		"record_count":         len(records),
		"invalid_run_count":    invalidCount,
		"invalid_run_classes":  invalidByClass,
		"denominator_policies": denominatorPolicies,
		"denominator_handling": map[string]any{
			"policy":           "exclude_invalid_runs",
			"included_records": len(records) - invalidCount,
			"excluded_records": invalidCount,
			"counted_policies": denominatorPolicies,
		},
	}
}

func buildRankClaimText(claims []fhiSubjectClaim) string {
	if len(claims) < 2 {
		return ""
	}
	leader := claims[0]
	runnerUp := claims[1]
	local := leader
	frontier := runnerUp
	if fhiStringPath(frontier.view, "runtime.deployment_class") == "local" || fhiStringPath(frontier.view, "runtime.local_runtime_version") != "" {
		local, frontier = frontier, local
	}
	localModel := firstNonEmptyString(fhiStringPath(local.view, "subject.model_raw"), fhiStringPath(local.view, "subject.model"))
	localRuntime := firstNonEmptyString(fhiStringPath(local.view, "runtime.local_runtime_name"), fhiStringPath(local.view, "subject.provider"))
	localRuntimeVersion := firstNonEmptyString(fhiStringPath(local.view, "runtime.local_runtime_version"), fhiStringPath(local.view, "provenance.provider_version"))
	frontierModel := firstNonEmptyString(fhiStringPath(frontier.view, "subject.model_raw"), fhiStringPath(frontier.view, "subject.model"))
	return fmt.Sprintf("With fiz %s and %s %s, %s gets FHI %.0f, %.0f points behind %s.",
		fhiStringPath(local.view, "provenance.fizeau_version"),
		localRuntime, localRuntimeVersion,
		localModel, roundToOne(local.score), abs(roundToOne(frontier.score-local.score)),
		frontierModel,
	)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func subjectKey(record map[string]any) string {
	subject := mapValue(record, "subject")
	if subject == nil {
		return ""
	}
	model := firstNonEmptyString(stringField(subject, "model"), stringField(subject, "model_raw"))
	harness := stringField(subject, "harness")
	provider := stringField(subject, "provider")
	return strings.Join([]string{model, harness, provider}, "\x00")
}

func stringOrEmpty(record map[string]any, path string) string {
	return fhiStringPath(record, path)
}

func mapValue(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

func fhiBoolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func fhiIntValue(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func intValueAny(v any) int {
	if out, ok := fhiIntValue(v); ok {
		return out
	}
	return 0
}

func fhiFloatValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		out, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return out, true
	default:
		return 0, false
	}
}

func fhiLookupPath(doc map[string]any, path string) (any, bool) {
	var cur any = doc
	for _, segment := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[segment]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func fhiStringPath(doc map[string]any, path string) string {
	value, ok := fhiLookupPath(doc, path)
	if !ok {
		return ""
	}
	s, _ := value.(string)
	return s
}
