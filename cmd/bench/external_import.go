package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DocumentDrivenDX/fizeau/internal/benchmark/evidence"
)

func cmdEvidenceImportExternal(args []string) int {
	fs := flagSet("evidence import-external")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	sourcePath := fs.String("source", "", "Curated external benchmark snapshot")
	outPath := fs.String("out", "", "Output JSONL path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*sourcePath) == "" || strings.TrimSpace(*outPath) == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s evidence import-external --source <fixture> --out <records.jsonl>\n", benchCommandName())
		return 2
	}

	wd := resolveWorkDir(*workDir)
	srcFile := resolveBenchmarkPath(wd, "", *sourcePath)
	outFile := resolveBenchmarkPath(wd, "", *outPath)

	records, err := importExternalBenchmarkRecords(wd, srcFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-external: %v\n", benchCommandName(), err)
		return 1
	}
	if len(records) == 0 {
		fmt.Fprintf(os.Stderr, "%s evidence import-external: no records parsed from %s\n", benchCommandName(), srcFile)
		return 1
	}
	for _, record := range records {
		recordID, err := evidence.StableRecordID(record)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s evidence import-external: compute record_id: %v\n", benchCommandName(), err)
			return 1
		}
		record["record_id"] = recordID
	}

	if err := os.MkdirAll(filepath.Dir(outFile), 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-external: create output dir: %v\n", benchCommandName(), err)
		return 1
	}
	tmp, err := os.CreateTemp(filepath.Dir(outFile), filepath.Base(outFile)+".*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-external: create temp output: %v\n", benchCommandName(), err)
		return 1
	}
	tmpName := tmp.Name()
	writeErr := writeBenchmarkEvidenceJSONL(tmp, records)
	closeErr := tmp.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-external: write output: %v\n", benchCommandName(), writeErr)
		return 1
	}

	validator, err := evidence.NewValidator(wd)
	if err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-external: %v\n", benchCommandName(), err)
		return 1
	}
	if _, err := validator.ValidateFile(tmpName); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-external: validate output: %v\n", benchCommandName(), err)
		return 1
	}
	if err := os.Rename(tmpName, outFile); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-external: rename output: %v\n", benchCommandName(), err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "%s evidence import-external: imported %d record(s) to %s\n", benchCommandName(), len(records), outFile)
	return 0
}

func importExternalBenchmarkRecords(workDir, sourceFile string) ([]map[string]any, error) {
	base := strings.ToLower(filepath.Base(sourceFile))
	ext := strings.ToLower(filepath.Ext(sourceFile))

	switch {
	case strings.HasSuffix(base, "rapid-mlx-mhi.md") || strings.Contains(base, "rapid-mlx") || strings.Contains(base, "mhi"):
		if ext != ".md" && ext != ".markdown" {
			return nil, fmt.Errorf("unsupported rapid-mlx MHI snapshot %s", sourceFile)
		}
		return importRapidMLXMHI(workDir, sourceFile)
	case strings.Contains(base, "skillsbench"):
		if ext != ".csv" {
			return nil, fmt.Errorf("unsupported SkillsBench snapshot %s", sourceFile)
		}
		return importSkillsBench(workDir, sourceFile)
	case strings.Contains(base, "swebench"):
		if ext != ".csv" {
			return nil, fmt.Errorf("unsupported SWE-bench snapshot %s", sourceFile)
		}
		return importSWEbench(workDir, sourceFile)
	case strings.Contains(base, "humaneval"):
		if ext != ".jsonl" {
			return nil, fmt.Errorf("unsupported HumanEval snapshot %s", sourceFile)
		}
		return importHumanEval(workDir, sourceFile)
	default:
		return nil, fmt.Errorf("cannot infer benchmark importer from %s; use filenames containing rapid-mlx, skillsbench, swebench, or humaneval", sourceFile)
	}
}

func importRapidMLXMHI(workDir, sourceFile string) ([]map[string]any, error) {
	meta, rows, err := parseMarkdownTableFixture(sourceFile)
	if err != nil {
		return nil, err
	}
	hash, err := sha256FileHex(sourceFile)
	if err != nil {
		return nil, err
	}

	records := make([]map[string]any, 0, len(rows))
	for i, row := range rows {
		record, err := buildRapidMLXMHIRecord(workDir, sourceFile, hash, meta, row, i+1)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildRapidMLXMHIRecord(workDir, sourceFile, sourceHash string, meta map[string]string, row map[string]string, rowNumber int) (map[string]any, error) {
	capturedAt := firstNonEmpty(meta["captured_at"], row["captured_at"])
	benchmarkVersion := firstNonEmpty(meta["benchmark_version"], row["benchmark_version"])
	if benchmarkVersion == "" {
		benchmarkVersion = "rapid-mlx@" + firstNonEmpty(meta["source_commit"], row["source_commit"])
	}
	record := externalEvidenceRecord(externalEvidenceInput{
		workDir:            workDir,
		sourceFile:         sourceFile,
		sourceHash:         sourceHash,
		sourceType:         firstNonEmpty(meta["source_type"], "imported_report"),
		sourceName:         firstNonEmpty(meta["source_name"], "rapid-mlx-mhi"),
		sourceURL:          firstNonEmpty(meta["source_url"], row["source_url"]),
		capturedAt:         capturedAt,
		benchmarkName:      firstNonEmpty(meta["benchmark_name"], "mhi"),
		benchmarkVersion:   benchmarkVersion,
		benchmarkDataset:   firstNonEmpty(meta["benchmark_dataset"], row["benchmark_dataset"]),
		benchmarkCommit:    firstNonEmpty(meta["source_commit"], row["source_commit"]),
		benchmarkScorer:    firstNonEmpty(meta["benchmark_scorer"], "Rapid-MLX README leaderboard"),
		modelRaw:           requiredField(row, "model_raw"),
		harness:            normalizeUnknown(row["harness"]),
		provider:           normalizeUnknown(firstNonEmpty(row["provider"], meta["provider"], "rapid-mlx")),
		scoreMetric:        firstNonEmpty(row["metric"], "mhi"),
		scoreValue:         parseFloatOrRequired(row["value"], "value"),
		scoreRawValue:      parseFloatOrRequired(row["raw_value"], "raw_value"),
		scoreN:             parseIntDefault(row["n"], 3),
		scorePassed:        parseIntDefault(row["passed"], 0),
		scoreFailed:        parseIntDefault(row["failed"], 0),
		scopeSubsetID:      firstNonEmpty(meta["subset_id"], row["subset_id"]),
		scopeSubsetVersion: firstNonEmpty(meta["subset_version"], row["subset_version"]),
		scopeRunID:         firstNonEmpty(meta["run_id"], row["run_id"], sourceFile),
		scopeTaskID:        firstNonEmpty(row["task_id"], ""),
		scopeRep:           parseIntDefault(row["rep"], 1),
		scopeNTasks:        parseIntDefault(row["n_tasks"], 0),
		coverageNote:       "imported Rapid-MLX MHI evidence is a model-harness composite, not a live Fizeau run",
		confidenceNote:     "imported from curated Rapid-MLX snapshot",
		fhiPrimary:         true,
		fhiRole:            "model_power_and_harness_composite",
	})
	if record["scope"].(map[string]any)["n_tasks"].(int) == 0 {
		record["scope"].(map[string]any)["n_tasks"] = 3
	}
	record["components"].(map[string]any)["mhi_score"] = parseFloatOrRequired(row["raw_value"], "raw_value")
	record["components"].(map[string]any)["source_row"] = rowNumber
	return record, nil
}

func importSkillsBench(workDir, sourceFile string) ([]map[string]any, error) {
	meta, rows, err := parseCSVFixture(sourceFile)
	if err != nil {
		return nil, err
	}
	hash, err := sha256FileHex(sourceFile)
	if err != nil {
		return nil, err
	}

	records := make([]map[string]any, 0, len(rows))
	for i, row := range rows {
		record, err := buildSkillsBenchRecord(workDir, sourceFile, hash, meta, row, i+1)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildSkillsBenchRecord(workDir, sourceFile, sourceHash string, meta map[string]string, row map[string]string, rowNumber int) (map[string]any, error) {
	withSkills := parseFloatOrRequired(row["with_skills_pass_rate"], "with_skills_pass_rate")
	withoutSkills := parseFloatDefault(row["without_skills_pass_rate"], 0)
	nTasks := parseIntDefault(row["n_tasks"], 84)
	record := externalEvidenceRecord(externalEvidenceInput{
		workDir:            workDir,
		sourceFile:         sourceFile,
		sourceHash:         sourceHash,
		sourceType:         firstNonEmpty(meta["source_type"], "external_leaderboard"),
		sourceName:         firstNonEmpty(meta["source_name"], "skillsbench"),
		sourceURL:          firstNonEmpty(meta["source_url"], "https://www.skillsbench.ai/"),
		capturedAt:         firstNonEmpty(meta["captured_at"], row["captured_at"]),
		benchmarkName:      firstNonEmpty(meta["benchmark_name"], "skillsbench"),
		benchmarkVersion:   firstNonEmpty(meta["benchmark_version"], "skillsbench@2026-05-06"),
		benchmarkDataset:   firstNonEmpty(meta["benchmark_dataset"], row["benchmark_dataset"]),
		benchmarkSubsetID:  firstNonEmpty(meta["subset_id"], row["subset_id"]),
		benchmarkScorer:    firstNonEmpty(meta["benchmark_scorer"], "SkillsBench leaderboard"),
		modelRaw:           requiredField(row, "model_raw"),
		harness:            normalizeUnknown(row["harness"]),
		provider:           normalizeUnknown(firstNonEmpty(row["provider"], meta["provider"])),
		scoreMetric:        "pass_rate",
		scoreValue:         withSkills,
		scoreRawValue:      roundPercent(withSkills),
		scoreN:             nTasks,
		scorePassed:        floatToCount(withSkills, nTasks),
		scoreFailed:        nTasks - floatToCount(withSkills, nTasks),
		scopeSubsetID:      firstNonEmpty(meta["subset_id"], row["subset_id"]),
		scopeSubsetVersion: firstNonEmpty(meta["subset_version"], row["subset_version"]),
		scopeRunID:         firstNonEmpty(meta["run_id"], row["run_id"], sourceFile),
		scopeTaskID:        firstNonEmpty(row["task_id"], ""),
		scopeRep:           parseIntDefault(row["rep"], 1),
		scopeNTasks:        nTasks,
		coverageNote:       "SkillsBench is a paired skills-uplift report; use with-skills and without-skills rows together for FHI analysis",
		confidenceNote:     "imported from curated SkillsBench snapshot",
		fhiPrimary:         true,
		fhiRole:            "skill_uplift_evidence",
	})
	record["components"].(map[string]any)["with_skills_pass_rate"] = withSkills
	record["components"].(map[string]any)["without_skills_pass_rate"] = withoutSkills
	record["components"].(map[string]any)["normalized_gain"] = parseFloatDefault(row["normalized_gain"], 0)
	record["components"].(map[string]any)["source_row"] = rowNumber
	record["coverage"].(map[string]any)["included_subsets"] = []string{firstNonEmpty(meta["subset_id"], row["subset_id"], "public-leaderboard")}
	return record, nil
}

func importSWEbench(workDir, sourceFile string) ([]map[string]any, error) {
	meta, rows, err := parseCSVFixture(sourceFile)
	if err != nil {
		return nil, err
	}
	hash, err := sha256FileHex(sourceFile)
	if err != nil {
		return nil, err
	}

	records := make([]map[string]any, 0, len(rows))
	for i, row := range rows {
		record, err := buildSWEbenchRecord(workDir, sourceFile, hash, meta, row, i+1)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildSWEbenchRecord(workDir, sourceFile, sourceHash string, meta map[string]string, row map[string]string, rowNumber int) (map[string]any, error) {
	recordType := strings.ToLower(firstNonEmpty(row["row_type"], row["kind"], "leaderboard"))
	scoreMetric := "resolved_rate"
	scoreValue := parseFloatOrRequired(row["resolved_rate"], "resolved_rate")
	scoreRaw := row["resolved_rate"]
	scoreN := parseIntDefault(row["n_instances"], 0)
	scorePassed := parseIntDefault(row["passed"], 0)
	scoreFailed := parseIntDefault(row["failed"], 0)
	scopeTaskID := row["task_id"]
	if recordType == "task" || scopeTaskID != "" {
		scoreMetric = "resolved"
		if row["resolved"] != "" {
			scoreValue = parseFloatOrRequired(row["resolved"], "resolved")
		} else {
			scoreValue = 1
		}
		scoreRaw = firstNonEmpty(row["resolved"], "resolved")
		if scoreN == 0 {
			scoreN = 1
		}
		if scorePassed == 0 && scoreValue > 0 {
			scorePassed = 1
		}
		if scoreFailed == 0 && scorePassed == 0 {
			scoreFailed = 1
		}
	}
	if recordType == "leaderboard" && scoreN == 0 {
		scoreN = scorePassed + scoreFailed
	}
	record := externalEvidenceRecord(externalEvidenceInput{
		workDir:            workDir,
		sourceFile:         sourceFile,
		sourceHash:         sourceHash,
		sourceType:         firstNonEmpty(meta["source_type"], "external_leaderboard"),
		sourceName:         firstNonEmpty(meta["source_name"], "swebench"),
		sourceURL:          firstNonEmpty(meta["source_url"], "https://www.swebench.com/"),
		capturedAt:         firstNonEmpty(meta["captured_at"], row["captured_at"]),
		benchmarkName:      firstNonEmpty(meta["benchmark_name"], "swe-bench"),
		benchmarkVersion:   firstNonEmpty(meta["benchmark_version"], "verified@2026-05-06"),
		benchmarkDataset:   firstNonEmpty(meta["benchmark_dataset"], row["benchmark_dataset"]),
		benchmarkSubsetID:  firstNonEmpty(meta["subset_id"], row["subset_id"]),
		benchmarkSubsetVer: firstNonEmpty(meta["subset_version"], row["subset_version"]),
		benchmarkScorer:    firstNonEmpty(meta["benchmark_scorer"], "SWE-bench leaderboard"),
		modelRaw:           requiredField(row, "model_raw"),
		harness:            normalizeUnknown(row["harness"]),
		provider:           normalizeUnknown(firstNonEmpty(row["provider"], meta["provider"])),
		scoreMetric:        scoreMetric,
		scoreValue:         scoreValue,
		scoreRawValue:      scoreRaw,
		scoreN:             scoreN,
		scorePassed:        scorePassed,
		scoreFailed:        scoreFailed,
		scopeSubsetID:      firstNonEmpty(meta["subset_id"], row["subset_id"]),
		scopeSubsetVersion: firstNonEmpty(meta["subset_version"], row["subset_version"]),
		scopeRunID:         firstNonEmpty(meta["run_id"], row["run_id"], sourceFile),
		scopeTaskID:        scopeTaskID,
		scopeRep:           parseIntDefault(row["rep"], 1),
		scopeNTasks:        parseIntDefault(row["n_instances"], 500),
		coverageNote:       "SWE-bench is a long-horizon coding signal; preserve harness and scaffold identity for FHI use",
		confidenceNote:     "imported from curated SWE-bench snapshot",
		fhiPrimary:         true,
		fhiRole:            "coding_agentic_evidence",
	})
	record["components"].(map[string]any)["row_type"] = recordType
	record["components"].(map[string]any)["source_row"] = rowNumber
	record["components"].(map[string]any)["task_repo"] = row["repo"]
	record["components"].(map[string]any)["task_language"] = row["language"]
	if row["task_id"] != "" {
		record["components"].(map[string]any)["task_id"] = row["task_id"]
	}
	return record, nil
}

func importHumanEval(workDir, sourceFile string) ([]map[string]any, error) {
	hash, err := sha256FileHex(sourceFile)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sourceFile, err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	records := make([]map[string]any, 0)
	rowNumber := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		rowNumber++
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", sourceFile, rowNumber, err)
		}
		record, err := buildHumanEvalRecord(workDir, sourceFile, hash, row, rowNumber)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func buildHumanEvalRecord(workDir, sourceFile, sourceHash string, row map[string]any, rowNumber int) (map[string]any, error) {
	capturedAt := anyString(row["captured_at"])
	benchmarkVersion := anyString(row["benchmark_version"])
	if benchmarkVersion == "" {
		benchmarkVersion = "openai-human-eval"
	}
	kind := strings.ToLower(firstNonEmpty(anyString(row["record_kind"]), anyString(row["kind"]), "aggregate"))
	record := externalEvidenceRecord(externalEvidenceInput{
		workDir:            workDir,
		sourceFile:         sourceFile,
		sourceHash:         sourceHash,
		sourceType:         firstNonEmpty(anyString(row["source_type"]), "imported_report"),
		sourceName:         firstNonEmpty(anyString(row["source_name"]), "humaneval"),
		sourceURL:          firstNonEmpty(anyString(row["source_url"]), "https://github.com/openai/human-eval"),
		capturedAt:         capturedAt,
		benchmarkName:      firstNonEmpty(anyString(row["benchmark_name"]), "humaneval"),
		benchmarkVersion:   benchmarkVersion,
		benchmarkDataset:   firstNonEmpty(anyString(row["dataset"]), anyString(row["benchmark_dataset"])),
		benchmarkSubsetID:  firstNonEmpty(anyString(row["subset_id"]), anyString(row["subset"])),
		benchmarkSubsetVer: firstNonEmpty(anyString(row["subset_version"]), anyString(row["dataset_version"])),
		benchmarkScorer:    firstNonEmpty(anyString(row["scorer"]), "HumanEval evaluator"),
		modelRaw:           requiredAnyField(row, "model_raw"),
		harness:            normalizeUnknown(anyString(row["harness"])),
		provider:           normalizeUnknown(firstNonEmpty(anyString(row["provider"]), "unknown")),
		scoreMetric:        "pass_at_1",
		scoreValue:         0,
		scoreRawValue:      nil,
		scoreN:             parseAnyInt(row["n"], 1),
		scorePassed:        0,
		scoreFailed:        0,
		scopeSubsetID:      firstNonEmpty(anyString(row["subset_id"]), anyString(row["subset"])),
		scopeSubsetVersion: firstNonEmpty(anyString(row["subset_version"]), anyString(row["dataset_version"])),
		scopeRunID:         firstNonEmpty(anyString(row["run_id"]), sourceFile),
		scopeTaskID:        anyString(row["task_id"]),
		scopeRep:           parseAnyInt(row["rep"], 1),
		scopeNTasks:        parseAnyInt(row["n_tasks"], 0),
		coverageNote:       "HumanEval is a low-cost model-power component and not a primary harness-intelligence benchmark",
		confidenceNote:     "imported from curated HumanEval snapshot",
		fhiPrimary:         false,
		fhiRole:            "model_power_component",
	})
	components := record["components"].(map[string]any)
	components["source_row"] = rowNumber
	components["record_kind"] = kind
	components["outcome"] = firstNonEmpty(anyString(row["outcome"]), kind)

	coverage := record["coverage"].(map[string]any)
	if kind == "result" {
		record["score"].(map[string]any)["metric"] = "passed"
		record["score"].(map[string]any)["value"] = boolToInt(firstNonEmpty(anyString(row["outcome"]), "failed") == "passed")
		record["score"].(map[string]any)["raw_value"] = firstNonEmpty(anyString(row["outcome"]), "failed")
		record["score"].(map[string]any)["n"] = 1
		record["score"].(map[string]any)["passed"] = boolToInt(firstNonEmpty(anyString(row["outcome"]), "failed") == "passed")
		record["score"].(map[string]any)["failed"] = boolToInt(firstNonEmpty(anyString(row["outcome"]), "failed") != "passed")
		record["scope"].(map[string]any)["task_id"] = anyString(row["task_id"])
		record["scope"].(map[string]any)["n_tasks"] = 1
		record["scope"].(map[string]any)["rep"] = parseAnyInt(row["rep"], 1)
		coverage["coverage_note"] = "completion-level HumanEval result imported for model-power analysis"
		return record, nil
	}

	passAt1 := parseAnyFloat(row["pass_at_1"], parseAnyFloat(row["value"], 0))
	scoreN := parseAnyInt(row["n"], 0)
	if scoreN == 0 {
		scoreN = 1
	}
	passed := parseAnyInt(row["passed"], 0)
	failed := parseAnyInt(row["failed"], 0)
	if passed == 0 && scoreN > 0 && passAt1 > 0 {
		passed = int(float64(scoreN) * passAt1)
	}
	if failed == 0 && scoreN >= passed {
		failed = scoreN - passed
	}
	record["score"].(map[string]any)["metric"] = "pass_at_1"
	record["score"].(map[string]any)["value"] = passAt1
	record["score"].(map[string]any)["raw_value"] = roundPercent(passAt1)
	record["score"].(map[string]any)["n"] = scoreN
	record["score"].(map[string]any)["passed"] = passed
	record["score"].(map[string]any)["failed"] = failed
	record["scope"].(map[string]any)["n_tasks"] = scoreN
	coverage["coverage_note"] = "aggregate HumanEval pass@1 row imported as low-cost model-power evidence"
	coverage["included_benchmarks"] = []string{"humaneval"}
	return record, nil
}

type externalEvidenceInput struct {
	workDir string

	sourceFile string
	sourceHash string

	sourceType string
	sourceName string
	sourceURL  string
	capturedAt string

	benchmarkName      string
	benchmarkVersion   string
	benchmarkDataset   string
	benchmarkCommit    string
	benchmarkSubsetID  string
	benchmarkSubsetVer string
	benchmarkScorer    string

	modelRaw      string
	harness       string
	provider      string
	scoreMetric   string
	scoreValue    float64
	scoreRawValue any
	scoreN        int
	scorePassed   int
	scoreFailed   int

	scopeRunID         string
	scopeTaskID        string
	scopeSubsetID      string
	scopeSubsetVersion string
	scopeRep           int
	scopeNTasks        int

	coverageNote   string
	confidenceNote string
	fhiPrimary     bool
	fhiRole        string
}

func externalEvidenceRecord(in externalEvidenceInput) map[string]any {
	record := map[string]any{
		"schema_version": evidence.SchemaVersion,
		"captured_at":    firstNonEmpty(in.capturedAt, "2026-05-06T00:00:00Z"),
		"source": map[string]any{
			"type":            in.sourceType,
			"name":            in.sourceName,
			"url":             in.sourceURL,
			"artifact_path":   relativeArtifactPath(in.workDir, in.sourceFile),
			"artifact_sha256": in.sourceHash,
		},
		"benchmark": map[string]any{
			"name":             in.benchmarkName,
			"version":          in.benchmarkVersion,
			"higher_is_better": true,
		},
		"subject": map[string]any{
			"model_raw": normalizeUnknown(in.modelRaw),
			"harness":   normalizeUnknown(in.harness),
			"provider":  normalizeUnknown(in.provider),
		},
		"scope": map[string]any{
			"run_id":           firstNonEmpty(in.scopeRunID, in.sourceFile),
			"task_id":          in.scopeTaskID,
			"subset_id":        firstNonEmpty(in.scopeSubsetID, in.benchmarkSubsetID),
			"subset_version":   firstNonEmpty(in.scopeSubsetVersion, in.benchmarkSubsetVer),
			"rep":              maxInt(in.scopeRep, 1),
			"n_tasks":          maxInt(in.scopeNTasks, 0),
			"denominator_rule": "count_valid_rows",
		},
		"score": map[string]any{
			"metric":    in.scoreMetric,
			"value":     in.scoreValue,
			"raw_value": in.scoreRawValue,
			"n":         maxInt(in.scoreN, 1),
			"passed":    maxInt(in.scorePassed, 0),
			"failed":    maxInt(in.scoreFailed, 0),
		},
		"coverage": map[string]any{
			"formula_version":     "fhi/v1",
			"evidence_window":     "2026-Q2",
			"included_benchmarks": []string{in.benchmarkName},
			"denominator_rule":    "count_valid_rows",
			"coverage_note":       in.coverageNote,
			"confidence_note":     in.confidenceNote,
		},
		"denominator": map[string]any{
			"included":         true,
			"policy":           "count_valid_rows",
			"reason":           "curated external benchmark snapshot imported into ledger",
			"included_count":   1,
			"excluded_count":   0,
			"excluded_classes": []string{},
		},
		"components": map[string]any{
			"fhi_primary": in.fhiPrimary,
			"fhi_role":    in.fhiRole,
		},
	}

	if in.benchmarkDataset != "" {
		record["benchmark"].(map[string]any)["dataset"] = in.benchmarkDataset
	}
	if in.benchmarkCommit != "" {
		record["benchmark"].(map[string]any)["dataset_commit"] = in.benchmarkCommit
	}
	if in.benchmarkSubsetID != "" {
		record["benchmark"].(map[string]any)["subset_id"] = in.benchmarkSubsetID
	}
	if in.benchmarkSubsetVer != "" {
		record["benchmark"].(map[string]any)["subset_version"] = in.benchmarkSubsetVer
	}
	if in.benchmarkScorer != "" {
		record["benchmark"].(map[string]any)["scorer"] = in.benchmarkScorer
	}
	if in.sourceURL == "" {
		record["source"].(map[string]any)["url"] = ""
	}
	return record
}

func parseMarkdownTableFixture(path string) (map[string]string, []map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	meta := map[string]string{}
	var tableLines []string
	inTable := false
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") && !inTable {
			key, value, ok := parseCommentKeyValue(line)
			if ok {
				meta[key] = value
			}
			continue
		}
		if strings.HasPrefix(line, "|") {
			inTable = true
			tableLines = append(tableLines, line)
		}
	}
	if len(tableLines) < 2 {
		return nil, nil, fmt.Errorf("%s does not contain a markdown table", path)
	}

	header := splitMarkdownRow(tableLines[0])
	if len(header) == 0 {
		return nil, nil, fmt.Errorf("%s has an empty markdown table header", path)
	}
	rows := make([]map[string]string, 0, len(tableLines)-2)
	for _, line := range tableLines[2:] {
		if isMarkdownSeparatorRow(line) {
			continue
		}
		values := splitMarkdownRow(line)
		if len(values) == 0 {
			continue
		}
		row := map[string]string{}
		for i, col := range header {
			if i < len(values) {
				row[strings.ToLower(col)] = values[i]
			}
		}
		rows = append(rows, row)
	}
	return meta, rows, nil
}

func splitMarkdownRow(line string) []string {
	trimmed := strings.TrimSpace(strings.Trim(line, "|"))
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func isMarkdownSeparatorRow(line string) bool {
	trimmed := strings.TrimSpace(strings.Trim(line, "|"))
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return true
}

func parseCSVFixture(path string) (map[string]string, []map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	meta := map[string]string{}
	var csvLines []string
	headerSeen := false
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if headerSeen {
				csvLines = append(csvLines, "")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") && !headerSeen {
			key, value, ok := parseCommentKeyValue(trimmed)
			if ok {
				meta[key] = value
			}
			continue
		}
		headerSeen = true
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		csvLines = append(csvLines, line)
	}
	if len(csvLines) == 0 {
		return nil, nil, fmt.Errorf("%s does not contain CSV rows", path)
	}

	reader := csv.NewReader(strings.NewReader(strings.Join(csvLines, "\n")))
	reader.FieldsPerRecord = -1
	rowsRaw, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(rowsRaw) < 2 {
		return nil, nil, fmt.Errorf("%s does not contain a CSV header and at least one row", path)
	}

	headers := rowsRaw[0]
	rows := make([]map[string]string, 0, len(rowsRaw)-1)
	for _, values := range rowsRaw[1:] {
		if len(values) == 1 && strings.TrimSpace(values[0]) == "" {
			continue
		}
		row := map[string]string{}
		for i, col := range headers {
			if i < len(values) {
				row[strings.ToLower(strings.TrimSpace(col))] = strings.TrimSpace(values[i])
			}
		}
		rows = append(rows, row)
	}
	return meta, rows, nil
}

func parseCommentKeyValue(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "#"))
	if trimmed == "" {
		return "", "", false
	}
	key, value, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value), true
}

func externalEvidenceString(row map[string]any, key string) string {
	return anyString(row[key])
}

func requiredField(row map[string]string, key string) string {
	v := strings.TrimSpace(row[strings.ToLower(key)])
	if v == "" {
		v = strings.TrimSpace(row[key])
	}
	if v == "" {
		return "unknown"
	}
	return v
}

func requiredAnyField(row map[string]any, key string) string {
	v := anyString(row[key])
	if v == "" {
		return "unknown"
	}
	return v
}

func normalizeUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseFloatOrRequired(value string, field string) float64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return f
}

func parseFloatDefault(value string, fallback float64) float64 {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return f
}

func parseIntDefault(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	i, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return i
}

func parseAnyFloat(value any, fallback float64) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f
		}
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return fallback
}

func parseAnyInt(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
		}
	}
	return fallback
}

func anyString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ""
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func roundPercent(value float64) float64 {
	return float64(int(value*100 + 0.5))
}

func floatToCount(rate float64, total int) int {
	if total <= 0 {
		return 0
	}
	if rate <= 0 {
		return 0
	}
	if rate >= 1 {
		return total
	}
	return int(rate * float64(total))
}

func maxInt(value int, fallback int) int {
	if value < fallback {
		return fallback
	}
	return value
}

func relativeArtifactPath(workDir, path string) string {
	if path == "" {
		return ""
	}
	if workDir == "" {
		return filepath.ToSlash(path)
	}
	if rel, err := filepath.Rel(workDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func sha256FileHex(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
