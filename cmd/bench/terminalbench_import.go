package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/evidence"
	"github.com/easel/fizeau/internal/benchmark/profile"
)

const terminalBenchMatrixMetadataName = "matrix.metadata.json"

func cmdEvidenceImportTerminalBench(args []string) int {
	fs := flagSet("evidence import-terminalbench")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	matrixDir := fs.String("matrix", "", "TerminalBench matrix output directory")
	outPath := fs.String("out", "", "Output JSONL path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*matrixDir) == "" || strings.TrimSpace(*outPath) == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s evidence import-terminalbench --matrix <dir> --out <records.jsonl>\n", benchCommandName())
		return 2
	}

	wd := resolveWorkDir(*workDir)
	matrixRoot := resolveMatrixPath(wd, *matrixDir)
	outFile := resolveMatrixPath(wd, *outPath)

	matrixOut, err := loadTerminalBenchMatrixOutput(matrixRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: load matrix: %v\n", benchCommandName(), err)
		return 1
	}

	metadata, _ := loadTerminalBenchMatrixMetadata(matrixRoot)
	subsetPath := resolveBenchmarkPath(wd, matrixRoot, matrixOut.SubsetPath)
	subset, err := loadTermbenchSubset(subsetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: load subset %s: %v\n", benchCommandName(), subsetPath, err)
		return 1
	}

	records := make([]map[string]any, 0, len(matrixOut.Runs))
	if len(matrixOut.Runs) > 0 {
		for _, run := range matrixOut.Runs {
			record, err := buildTerminalBenchRecord(wd, matrixRoot, matrixOut, subset, metadata, run)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: %v\n", benchCommandName(), err)
				return 1
			}
			records = append(records, record)
		}
	} else if len(matrixOut.Cells) > 0 {
		for _, cell := range matrixOut.Cells {
			record, err := buildTerminalBenchAggregateRecord(matrixRoot, matrixOut, subset, metadata, cell)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: %v\n", benchCommandName(), err)
				return 1
			}
			records = append(records, record)
		}
	} else {
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: matrix.json contains no runs or cells\n", benchCommandName())
		return 1
	}

	if err := os.MkdirAll(filepath.Dir(outFile), 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: create output dir: %v\n", benchCommandName(), err)
		return 1
	}
	tmp, err := os.CreateTemp(filepath.Dir(outFile), filepath.Base(outFile)+".*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: create temp output: %v\n", benchCommandName(), err)
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
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: write output: %v\n", benchCommandName(), writeErr)
		return 1
	}

	validator, err := evidence.NewValidator(wd)
	if err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: %v\n", benchCommandName(), err)
		return 1
	}
	if _, err := validator.ValidateFile(tmpName); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: validate output: %v\n", benchCommandName(), err)
		return 1
	}
	if err := os.Rename(tmpName, outFile); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: rename output: %v\n", benchCommandName(), err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "%s evidence import-terminalbench: imported %d record(s) to %s\n", benchCommandName(), len(records), outFile)
	return 0
}

func loadTerminalBenchMatrixOutput(matrixRoot string) (*matrixOutput, error) {
	raw, err := os.ReadFile(filepath.Join(matrixRoot, "matrix.json"))
	if err != nil {
		return nil, fmt.Errorf("read matrix.json: %w", err)
	}
	var out matrixOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse matrix.json: %w", err)
	}
	return &out, nil
}

func loadTerminalBenchMatrixMetadata(matrixRoot string) (map[string]any, error) {
	path := filepath.Join(matrixRoot, terminalBenchMatrixMetadataName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

func buildTerminalBenchRecord(workDir, matrixRoot string, matrix *matrixOutput, subset *termbenchSubset, metadata map[string]any, run matrixRunReport) (map[string]any, error) {
	profilePath := resolveBenchmarkPath(workDir, matrixRoot, run.ProfilePath)
	prof, err := profile.Load(profilePath)
	if err != nil {
		return nil, fmt.Errorf("load profile %s: %w", profilePath, err)
	}

	record := terminalBenchBaseRecord(matrixRoot, matrix, subset, metadata, run, prof)
	record["source"] = terminalBenchSourceRecord(matrixRoot, "matrix.json", run.OutputDir)
	record["score"] = terminalBenchScoreRecord(run, 0)
	record["final_status"] = run.FinalStatus
	if invalidClass := classifyMatrixInvalid(run); invalidClass != "" {
		record["invalid_class"] = invalidClass
		record["scope"].(map[string]any)["denominator_rule"] = "exclude_invalid_runs"
		record["denominator"] = terminalBenchDenominatorRecord(run, invalidClass, false)
		record["coverage"] = terminalBenchCoverageRecord(matrix, subset, metadata, run, invalidClass, false)
	} else {
		record["scope"].(map[string]any)["denominator_rule"] = "count_valid_tasks"
		record["denominator"] = terminalBenchDenominatorRecord(run, "", true)
		record["coverage"] = terminalBenchCoverageRecord(matrix, subset, metadata, run, "", true)
	}

	if path, hash := terminalBenchArtifactHash(matrixRoot, run.OutputDir, "logs", "agent", "session.log.jsonl"); path != "" {
		record["provenance"].(map[string]any)["session_log_path"] = path
		record["provenance"].(map[string]any)["session_log_sha256"] = hash
	}
	if path, hash := terminalBenchArtifactHash(matrixRoot, run.OutputDir, "logs", "agent", "trajectory.json"); path != "" {
		record["provenance"].(map[string]any)["trajectory_path"] = path
		record["provenance"].(map[string]any)["trajectory_sha256"] = hash
	}
	if path, hash := terminalBenchArtifactHash(matrixRoot, run.OutputDir, "logs", "verifier", "reward.txt"); path != "" {
		record["components"].(map[string]any)["reward_file_path"] = path
		record["components"].(map[string]any)["reward_file_sha256"] = hash
	}

	if run.Reward == nil {
		if reward, err := terminalBenchRewardFromFile(matrixRoot, run.OutputDir); err == nil {
			record["score"] = terminalBenchScoreRecord(run, reward)
		}
	}
	if run.Reward != nil {
		record["score"] = terminalBenchScoreRecord(run, *run.Reward)
	}

	if err := terminalBenchApplyMetadata(record, metadata, prof, run); err != nil {
		return nil, err
	}

	recordID, err := evidence.StableRecordID(record)
	if err != nil {
		return nil, err
	}
	record["record_id"] = recordID
	return record, nil
}

func buildTerminalBenchAggregateRecord(matrixRoot string, matrix *matrixOutput, subset *termbenchSubset, metadata map[string]any, cell matrixCell) (map[string]any, error) {
	record := map[string]any{
		"schema_version": evidence.SchemaVersion,
		"captured_at":    stringOrDefault(metadataPathString(metadata, "captured_at"), time.Now().UTC().Format(time.RFC3339)),
		"source":         terminalBenchSourceRecord(matrixRoot, "matrix.json", "matrix.json"),
		"benchmark":      terminalBenchBenchmarkRecord(matrix, subset, metadata),
		"subject": map[string]any{
			"harness":   cell.Harness,
			"provider":  metadataPathString(metadata, "subject.provider"),
			"model_raw": metadataPathString(metadata, "subject.model_raw"),
		},
		"scope": map[string]any{
			"run_id":           cell.Harness + "/" + cell.ProfileID,
			"subset":           matrixSubsetID(matrix, metadata, subset),
			"subset_id":        matrixSubsetID(matrix, metadata, subset),
			"subset_version":   matrixSubsetVersion(metadata, subset),
			"denominator_rule": "count_valid_tasks",
			"split":            metadataPathString(metadata, "scope.split"),
			"n_tasks":          len(subset.Tasks),
			"task_id":          "",
			"rep":              0,
		},
		"score": map[string]any{
			"metric":    "pass_rate",
			"value":     cellScoreValue(cell),
			"raw_value": cellScoreValue(cell),
			"n":         cell.NRuns,
			"passed":    cell.NValid,
			"failed":    cell.NInvalid,
		},
		"coverage": map[string]any{
			"formula_version":     stringOrDefault(metadataPathString(metadata, "coverage.formula_version"), "fhi/v1"),
			"evidence_window":     stringOrDefault(metadataPathString(metadata, "coverage.evidence_window"), "2026-Q2"),
			"included_benchmarks": []string{"terminal-bench"},
			"included_subsets":    []string{matrixSubsetID(matrix, metadata, subset)},
			"denominator_rule":    "count_valid_tasks",
			"coverage_note":       "aggregate matrix cell imported without task-level rows",
			"confidence_note":     "aggregate record imported from matrix.json cells",
		},
		"denominator": map[string]any{
			"included":         true,
			"policy":           "count_valid_runs_only",
			"reason":           "aggregate cell imported from matrix summary",
			"included_count":   cell.NValid,
			"excluded_count":   cell.NInvalid,
			"excluded_classes": terminalBenchAggregateExcludedClasses(cell),
		},
		"components": map[string]any{
			"matrix_cell_harness":  cell.Harness,
			"matrix_cell_profile":  cell.ProfileID,
			"matrix_cell_cost_usd": cell.CostUSD,
		},
	}
	recordID, err := evidence.StableRecordID(record)
	if err != nil {
		return nil, err
	}
	record["record_id"] = recordID
	return record, nil
}

func terminalBenchBaseRecord(matrixRoot string, matrix *matrixOutput, subset *termbenchSubset, metadata map[string]any, run matrixRunReport, prof *profile.Profile) map[string]any {
	subsetID := matrixSubsetID(matrix, metadata, subset)
	subsetVersion := matrixSubsetVersion(metadata, subset)
	capturedAt := metadataPathString(metadata, "captured_at")
	if capturedAt == "" {
		capturedAt = run.FinishedAt.UTC().Format(time.RFC3339)
	}

	record := map[string]any{
		"schema_version": evidence.SchemaVersion,
		"captured_at":    capturedAt,
		"source":         map[string]any{},
		"benchmark":      terminalBenchBenchmarkRecord(matrix, subset, metadata),
		"subject": map[string]any{
			"model_raw": prof.Provider.Model,
			"harness":   run.Harness,
			"provider":  terminalBenchProviderName(prof),
			"surface":   terminalBenchProviderSurface(prof),
		},
		"scope": map[string]any{
			"run_id":           strings.TrimSpace(run.OutputDir),
			"task_id":          run.TaskID,
			"subset":           subsetID,
			"subset_id":        subsetID,
			"subset_version":   subsetVersion,
			"rep":              run.Rep,
			"n_tasks":          len(subset.Tasks),
			"split":            stringOrDefault(metadataPathString(metadata, "scope.split"), "test"),
			"denominator_rule": "count_valid_tasks",
		},
		"score": map[string]any{},
		"coverage": map[string]any{
			"formula_version":     stringOrDefault(metadataPathString(metadata, "coverage.formula_version"), "fhi/v1"),
			"evidence_window":     stringOrDefault(metadataPathString(metadata, "coverage.evidence_window"), "2026-Q2"),
			"included_benchmarks": []string{"terminal-bench"},
			"included_subsets":    []string{subsetID},
			"denominator_rule":    "count_valid_tasks",
		},
		"denominator": map[string]any{
			"included":         true,
			"policy":           "count_valid_runs_only",
			"reason":           "task-level record reached grading",
			"included_count":   1,
			"excluded_count":   0,
			"excluded_classes": []string{},
		},
		"runtime":    map[string]any{},
		"provenance": terminalBenchProvenanceRecord(matrixRoot, run, prof, metadata),
		"components": map[string]any{
			"matrix_output_dir": matrixRoot,
			"matrix_profile_id": run.ProfileID,
		},
	}

	if surface := terminalBenchProviderSurface(prof); surface != "" {
		record["subject"].(map[string]any)["surface"] = surface
	}
	if endpoint := strings.TrimSpace(prof.Provider.BaseURL); endpoint != "" {
		record["subject"].(map[string]any)["endpoint"] = endpoint
		record["provenance"].(map[string]any)["provider_endpoint"] = endpoint
	}
	if model := strings.TrimSpace(metadataPathString(metadata, "subject.model")); model != "" {
		record["subject"].(map[string]any)["model"] = model
	}
	if deployment := metadataPathString(metadata, "runtime.deployment_class"); deployment != "" {
		record["subject"].(map[string]any)["deployment_class"] = deployment
		record["runtime"].(map[string]any)["deployment_class"] = deployment
	}
	if reasoning := metadataPathString(metadata, "subject.reasoning"); reasoning != "" {
		record["subject"].(map[string]any)["reasoning"] = reasoning
	}
	if quantization := metadataPathString(metadata, "runtime.quantization"); quantization != "" {
		record["runtime"].(map[string]any)["quantization"] = quantization
	}
	if localRuntimeName := metadataPathString(metadata, "runtime.local_runtime_name"); localRuntimeName != "" {
		record["runtime"].(map[string]any)["local_runtime_name"] = localRuntimeName
	}
	if localRuntimeVersion := metadataPathString(metadata, "runtime.local_runtime_version"); localRuntimeVersion != "" {
		record["runtime"].(map[string]any)["local_runtime_version"] = localRuntimeVersion
	}
	if hardwareClass := metadataPathString(metadata, "runtime.hardware_class"); hardwareClass != "" {
		record["runtime"].(map[string]any)["hardware_class"] = hardwareClass
	}
	if hardwareAccelerator := metadataPathString(metadata, "runtime.hardware_accelerator"); hardwareAccelerator != "" {
		record["runtime"].(map[string]any)["hardware_accelerator"] = hardwareAccelerator
	}
	if hardwareBackend := metadataPathString(metadata, "runtime.hardware_accelerator_backend"); hardwareBackend != "" {
		record["runtime"].(map[string]any)["hardware_accelerator_backend"] = hardwareBackend
	}
	if hardwareMemory := metadataPathFloat(metadata, "runtime.hardware_memory_gb"); hardwareMemory > 0 {
		record["runtime"].(map[string]any)["hardware_memory_gb"] = hardwareMemory
	}
	if hardwareOS := metadataPathString(metadata, "runtime.hardware_os"); hardwareOS != "" {
		record["runtime"].(map[string]any)["hardware_os"] = hardwareOS
	}
	if hardwareArch := metadataPathString(metadata, "runtime.hardware_arch"); hardwareArch != "" {
		record["runtime"].(map[string]any)["hardware_arch"] = hardwareArch
	}
	if harnessWrapperName := metadataPathString(metadata, "provenance.harness_wrapper_name"); harnessWrapperName != "" {
		record["provenance"].(map[string]any)["harness_wrapper_name"] = harnessWrapperName
	}
	if harnessWrapperVersion := metadataPathString(metadata, "provenance.harness_wrapper_version"); harnessWrapperVersion != "" {
		record["provenance"].(map[string]any)["harness_wrapper_version"] = harnessWrapperVersion
	}
	if harnessCLIVersion := metadataPathString(metadata, "provenance.harness_cli_version"); harnessCLIVersion != "" {
		record["provenance"].(map[string]any)["harness_cli_version"] = harnessCLIVersion
	}
	if harnessRuntimeVersion := metadataPathString(metadata, "provenance.harness_runtime_version"); harnessRuntimeVersion != "" {
		record["provenance"].(map[string]any)["harness_runtime_version"] = harnessRuntimeVersion
	}
	if fizeauVersion := metadataPathString(metadata, "provenance.fizeau_version"); fizeauVersion != "" {
		record["provenance"].(map[string]any)["fizeau_version"] = fizeauVersion
	}
	if fizeauCommit := metadataPathString(metadata, "provenance.fizeau_git_commit"); fizeauCommit != "" {
		record["provenance"].(map[string]any)["fizeau_git_commit"] = fizeauCommit
	}
	if providerVersion := metadataPathString(metadata, "provenance.provider_version"); providerVersion != "" {
		record["provenance"].(map[string]any)["provider_version"] = providerVersion
	}
	if providerCaptureAt := metadataPathString(metadata, "provenance.provider_capture_at"); providerCaptureAt != "" {
		record["provenance"].(map[string]any)["provider_capture_at"] = providerCaptureAt
	}
	if modelSnapshot := metadataPathString(metadata, "provenance.model_snapshot"); modelSnapshot != "" {
		record["provenance"].(map[string]any)["model_snapshot"] = modelSnapshot
	}
	if modelVersion := metadataPathString(metadata, "provenance.model_version"); modelVersion != "" {
		record["provenance"].(map[string]any)["model_version"] = modelVersion
	}
	if benchmarkRunnerVersion := metadataPathString(metadata, "provenance.benchmark_runner_version"); benchmarkRunnerVersion != "" {
		record["provenance"].(map[string]any)["benchmark_runner_version"] = benchmarkRunnerVersion
	}
	if benchmarkSubset := metadataPathString(metadata, "provenance.benchmark_subset"); benchmarkSubset != "" {
		record["provenance"].(map[string]any)["benchmark_subset"] = benchmarkSubset
	} else {
		record["provenance"].(map[string]any)["benchmark_subset"] = subsetID
	}
	if benchmarkSubsetVersion := metadataPathString(metadata, "provenance.benchmark_subset_version"); benchmarkSubsetVersion != "" {
		record["provenance"].(map[string]any)["benchmark_subset_version"] = benchmarkSubsetVersion
	} else {
		record["provenance"].(map[string]any)["benchmark_subset_version"] = subsetVersion
	}

	record["subject"].(map[string]any)["provider"] = terminalBenchProviderName(prof)
	if run.ModelServerInfo != nil {
		if run.ModelServerInfo.Quantization != "" {
			record["runtime"].(map[string]any)["quantization"] = run.ModelServerInfo.Quantization
		}
		if run.ModelServerInfo.LoadedContextLength > 0 {
			record["runtime"].(map[string]any)["context_limit"] = run.ModelServerInfo.LoadedContextLength
		}
	}

	record["score"] = terminalBenchScoreRecord(run, 0)
	if run.Reward != nil {
		record["score"] = terminalBenchScoreRecord(run, *run.Reward)
	}
	record["runtime"].(map[string]any)["wall_seconds"] = runDurationSeconds(run)
	record["runtime"].(map[string]any)["turns"] = intValue(run.Turns)
	record["runtime"].(map[string]any)["tool_calls"] = intValue(run.ToolCalls)
	record["runtime"].(map[string]any)["tool_call_errors"] = intValue(run.ToolCallErrors)
	record["runtime"].(map[string]any)["exit_code"] = run.ExitCode
	if run.FinalStatus != "" {
		record["runtime"].(map[string]any)["outcome"] = run.FinalStatus
	}

	if metadataPathBool(metadata, "runtime.deployments.local_only") {
		record["runtime"].(map[string]any)["deployment_class"] = "local"
	}
	return record
}

func terminalBenchApplyMetadata(record map[string]any, metadata map[string]any, prof *profile.Profile, run matrixRunReport) error {
	if metadata == nil {
		return nil
	}
	if providerVersion := metadataPathString(metadata, "provenance.provider_version"); providerVersion != "" {
		record["provenance"].(map[string]any)["provider_version"] = providerVersion
	}
	if providerCaptureAt := metadataPathString(metadata, "provenance.provider_capture_at"); providerCaptureAt != "" {
		record["provenance"].(map[string]any)["provider_capture_at"] = providerCaptureAt
	}
	if modelSnapshot := metadataPathString(metadata, "provenance.model_snapshot"); modelSnapshot != "" {
		record["provenance"].(map[string]any)["model_snapshot"] = modelSnapshot
	} else if prof.Versioning.Snapshot != "" {
		record["provenance"].(map[string]any)["model_snapshot"] = prof.Versioning.Snapshot
	}
	if modelVersion := metadataPathString(metadata, "provenance.model_version"); modelVersion != "" {
		record["provenance"].(map[string]any)["model_version"] = modelVersion
	}
	if fizeauVersion := metadataPathString(metadata, "provenance.fizeau_version"); fizeauVersion != "" {
		record["provenance"].(map[string]any)["fizeau_version"] = fizeauVersion
	}
	if fizeauCommit := metadataPathString(metadata, "provenance.fizeau_git_commit"); fizeauCommit != "" {
		record["provenance"].(map[string]any)["fizeau_git_commit"] = fizeauCommit
	}
	if harnessWrapperName := metadataPathString(metadata, "provenance.harness_wrapper_name"); harnessWrapperName != "" {
		record["provenance"].(map[string]any)["harness_wrapper_name"] = harnessWrapperName
	} else {
		record["provenance"].(map[string]any)["harness_wrapper_name"] = terminalBenchHarnessWrapperName(run.Harness)
	}
	if harnessWrapperVersion := metadataPathString(metadata, "provenance.harness_wrapper_version"); harnessWrapperVersion != "" {
		record["provenance"].(map[string]any)["harness_wrapper_version"] = harnessWrapperVersion
	}
	if harnessCLIVersion := metadataPathString(metadata, "provenance.harness_cli_version"); harnessCLIVersion != "" {
		record["provenance"].(map[string]any)["harness_cli_version"] = harnessCLIVersion
	}
	if harnessRuntimeVersion := metadataPathString(metadata, "provenance.harness_runtime_version"); harnessRuntimeVersion != "" {
		record["provenance"].(map[string]any)["harness_runtime_version"] = harnessRuntimeVersion
	}
	if benchmarkRunnerVersion := metadataPathString(metadata, "provenance.benchmark_runner_version"); benchmarkRunnerVersion != "" {
		record["provenance"].(map[string]any)["benchmark_runner_version"] = benchmarkRunnerVersion
	}
	if benchmarkSubset := metadataPathString(metadata, "provenance.benchmark_subset"); benchmarkSubset != "" {
		record["provenance"].(map[string]any)["benchmark_subset"] = benchmarkSubset
	}
	if benchmarkSubsetVersion := metadataPathString(metadata, "provenance.benchmark_subset_version"); benchmarkSubsetVersion != "" {
		record["provenance"].(map[string]any)["benchmark_subset_version"] = benchmarkSubsetVersion
	}
	if deployment := metadataPathString(metadata, "runtime.deployment_class"); deployment != "" {
		record["runtime"].(map[string]any)["deployment_class"] = deployment
	}
	if quantization := metadataPathString(metadata, "runtime.quantization"); quantization != "" {
		record["runtime"].(map[string]any)["quantization"] = quantization
	}
	if localRuntimeName := metadataPathString(metadata, "runtime.local_runtime_name"); localRuntimeName != "" {
		record["runtime"].(map[string]any)["local_runtime_name"] = localRuntimeName
	}
	if localRuntimeVersion := metadataPathString(metadata, "runtime.local_runtime_version"); localRuntimeVersion != "" {
		record["runtime"].(map[string]any)["local_runtime_version"] = localRuntimeVersion
	}
	if hardwareClass := metadataPathString(metadata, "runtime.hardware_class"); hardwareClass != "" {
		record["runtime"].(map[string]any)["hardware_class"] = hardwareClass
	}
	if hardwareAccelerator := metadataPathString(metadata, "runtime.hardware_accelerator"); hardwareAccelerator != "" {
		record["runtime"].(map[string]any)["hardware_accelerator"] = hardwareAccelerator
	}
	if hardwareBackend := metadataPathString(metadata, "runtime.hardware_accelerator_backend"); hardwareBackend != "" {
		record["runtime"].(map[string]any)["hardware_accelerator_backend"] = hardwareBackend
	}
	if hardwareMemory := metadataPathFloat(metadata, "runtime.hardware_memory_gb"); hardwareMemory > 0 {
		record["runtime"].(map[string]any)["hardware_memory_gb"] = hardwareMemory
	}
	if hardwareOS := metadataPathString(metadata, "runtime.hardware_os"); hardwareOS != "" {
		record["runtime"].(map[string]any)["hardware_os"] = hardwareOS
	}
	if hardwareArch := metadataPathString(metadata, "runtime.hardware_arch"); hardwareArch != "" {
		record["runtime"].(map[string]any)["hardware_arch"] = hardwareArch
	}
	if endpoint := strings.TrimSpace(prof.Provider.BaseURL); endpoint != "" {
		record["subject"].(map[string]any)["endpoint"] = endpoint
		record["provenance"].(map[string]any)["provider_endpoint"] = endpoint
	}
	record["subject"].(map[string]any)["provider"] = terminalBenchProviderName(prof)
	record["subject"].(map[string]any)["surface"] = terminalBenchProviderSurface(prof)
	if model := metadataPathString(metadata, "subject.model"); model != "" {
		record["subject"].(map[string]any)["model"] = model
	}
	if reasoning := metadataPathString(metadata, "subject.reasoning"); reasoning != "" {
		record["subject"].(map[string]any)["reasoning"] = reasoning
	}
	if deployment := metadataPathString(metadata, "subject.deployment_class"); deployment != "" {
		record["subject"].(map[string]any)["deployment_class"] = deployment
	}
	return nil
}

func terminalBenchBenchmarkRecord(matrix *matrixOutput, subset *termbenchSubset, metadata map[string]any) map[string]any {
	record := map[string]any{
		"name":             "terminal-bench",
		"version":          stringOrDefault(metadataPathString(metadata, "benchmark.version"), matrix.GeneratedAt.UTC().Format("2006.01.02")),
		"dataset_commit":   stringOrDefault(metadataPathString(metadata, "benchmark.dataset_commit"), subset.DatasetCommit),
		"subset_id":        matrixSubsetID(matrix, metadata, subset),
		"subset_version":   matrixSubsetVersion(metadata, subset),
		"scorer":           stringOrDefault(metadataPathString(metadata, "benchmark.scorer"), "verifier"),
		"scorer_version":   stringOrDefault(metadataPathString(metadata, "benchmark.scorer_version"), "1.0.0"),
		"higher_is_better": boolOrDefault(metadataPathBool(metadata, "benchmark.higher_is_better"), true),
	}
	if dataset := metadataPathString(metadata, "benchmark.dataset"); dataset != "" {
		record["dataset"] = dataset
	} else {
		record["dataset"] = subset.Dataset
	}
	return record
}

func terminalBenchProvenanceRecord(matrixRoot string, run matrixRunReport, prof *profile.Profile, metadata map[string]any) map[string]any {
	record := map[string]any{}
	if fizeauVersion := metadataPathString(metadata, "provenance.fizeau_version"); fizeauVersion != "" {
		record["fizeau_version"] = fizeauVersion
	}
	if fizeauCommit := metadataPathString(metadata, "provenance.fizeau_git_commit"); fizeauCommit != "" {
		record["fizeau_git_commit"] = fizeauCommit
	}
	if harnessWrapperName := metadataPathString(metadata, "provenance.harness_wrapper_name"); harnessWrapperName != "" {
		record["harness_wrapper_name"] = harnessWrapperName
	} else {
		record["harness_wrapper_name"] = terminalBenchHarnessWrapperName(run.Harness)
	}
	if harnessWrapperVersion := metadataPathString(metadata, "provenance.harness_wrapper_version"); harnessWrapperVersion != "" {
		record["harness_wrapper_version"] = harnessWrapperVersion
	}
	if harnessCLIVersion := metadataPathString(metadata, "provenance.harness_cli_version"); harnessCLIVersion != "" {
		record["harness_cli_version"] = harnessCLIVersion
	}
	if harnessRuntimeVersion := metadataPathString(metadata, "provenance.harness_runtime_version"); harnessRuntimeVersion != "" {
		record["harness_runtime_version"] = harnessRuntimeVersion
	}
	if providerEndpoint := strings.TrimSpace(prof.Provider.BaseURL); providerEndpoint != "" {
		record["provider_endpoint"] = providerEndpoint
	}
	if providerSurface := terminalBenchProviderSurface(prof); providerSurface != "" {
		record["provider_surface"] = providerSurface
	}
	if providerVersion := metadataPathString(metadata, "provenance.provider_version"); providerVersion != "" {
		record["provider_version"] = providerVersion
	}
	if providerCaptureAt := metadataPathString(metadata, "provenance.provider_capture_at"); providerCaptureAt != "" {
		record["provider_capture_at"] = providerCaptureAt
	}
	if modelSnapshot := metadataPathString(metadata, "provenance.model_snapshot"); modelSnapshot != "" {
		record["model_snapshot"] = modelSnapshot
	} else if prof.Versioning.Snapshot != "" {
		record["model_snapshot"] = prof.Versioning.Snapshot
	}
	if modelVersion := metadataPathString(metadata, "provenance.model_version"); modelVersion != "" {
		record["model_version"] = modelVersion
	}
	if benchmarkSubset := metadataPathString(metadata, "provenance.benchmark_subset"); benchmarkSubset != "" {
		record["benchmark_subset"] = benchmarkSubset
	}
	if benchmarkSubsetVersion := metadataPathString(metadata, "provenance.benchmark_subset_version"); benchmarkSubsetVersion != "" {
		record["benchmark_subset_version"] = benchmarkSubsetVersion
	}
	if benchmarkRunnerVersion := metadataPathString(metadata, "provenance.benchmark_runner_version"); benchmarkRunnerVersion != "" {
		record["benchmark_runner_version"] = benchmarkRunnerVersion
	}
	return record
}

func terminalBenchSourceRecord(matrixRoot, artifactPath, runDir string) map[string]any {
	relPath := artifactPath
	if relPath == "" {
		relPath = "matrix.json"
	}
	absPath := resolveBenchmarkPath("", matrixRoot, relPath)
	sum := sha256File(absPath)
	record := map[string]any{
		"type":          "imported_report",
		"name":          "TerminalBench matrix import",
		"artifact_path": filepath.ToSlash(relPath),
	}
	if sum != "" {
		record["artifact_sha256"] = sum
	}
	if runDir != "" {
		record["notes"] = []string{filepath.ToSlash(runDir)}
	}
	return record
}

func terminalBenchCoverageRecord(matrix *matrixOutput, subset *termbenchSubset, metadata map[string]any, run matrixRunReport, invalidClass string, included bool) map[string]any {
	note := "task-level TerminalBench matrix record imported from matrix.json"
	if !included {
		note = "invalid benchmark run excluded from pass-rate denominator"
	}
	confidence := "session and trajectory hashes present"
	if invalidClass != "" {
		confidence = "invalid run excluded from capability denominators"
	}
	return map[string]any{
		"formula_version":     stringOrDefault(metadataPathString(metadata, "coverage.formula_version"), "fhi/v1"),
		"evidence_window":     stringOrDefault(metadataPathString(metadata, "coverage.evidence_window"), "2026-Q2"),
		"included_benchmarks": []string{"terminal-bench"},
		"included_subsets":    []string{matrixSubsetID(matrix, metadata, subset)},
		"denominator_rule": func() string {
			if included {
				return "count_valid_tasks"
			}
			return "exclude_invalid_runs"
		}(),
		"coverage_note":   note,
		"confidence_note": confidence,
	}
}

func terminalBenchDenominatorRecord(run matrixRunReport, invalidClass string, included bool) map[string]any {
	if included {
		return map[string]any{
			"included":         true,
			"policy":           "count_valid_runs_only",
			"reason":           "task reached grading",
			"included_count":   1,
			"excluded_count":   0,
			"excluded_classes": []string{},
		}
	}
	return map[string]any{
		"included":         false,
		"policy":           "exclude_invalid_runs",
		"reason":           terminalBenchInvalidReason(invalidClass),
		"excluded_classes": []string{invalidClass},
		"excluded_tasks":   []string{run.TaskID},
		"included_count":   0,
		"excluded_count":   1,
	}
}

func terminalBenchScoreRecord(run matrixRunReport, reward int) map[string]any {
	passed := 0
	failed := 0
	if reward >= 1 {
		passed = 1
	} else {
		failed = 1
	}
	return map[string]any{
		"metric":    "pass_rate",
		"value":     float64(reward),
		"raw_value": reward,
		"n":         1,
		"passed":    passed,
		"failed":    failed,
	}
}

func terminalBenchArtifactHash(matrixRoot, runDir string, parts ...string) (string, string) {
	if runDir == "" {
		return "", ""
	}
	rel := filepath.Join(append([]string{runDir}, parts...)...)
	abs := resolveBenchmarkPath("", matrixRoot, rel)
	if abs == "" {
		return "", ""
	}
	if _, err := os.Stat(abs); err != nil {
		return "", ""
	}
	return filepath.ToSlash(rel), sha256File(abs)
}

func terminalBenchRewardFromFile(matrixRoot, runDir string) (int, error) {
	path := resolveBenchmarkPath("", matrixRoot, filepath.Join(runDir, "logs", "verifier", "reward.txt"))
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(raw)))
}

func terminalBenchInvalidReason(class string) string {
	switch class {
	case matrixInvalidQuota:
		return "quota exhausted before benchmark task"
	case matrixInvalidAuth:
		return "authentication failure before benchmark task"
	case matrixInvalidSetup:
		return "setup failure before first benchmark task"
	case matrixInvalidProvider:
		return "provider transport failure before benchmark task"
	default:
		return "invalid benchmark run excluded from pass-rate denominator"
	}
}

func terminalBenchProviderName(prof *profile.Profile) string {
	switch prof.Provider.Type {
	case profile.ProviderOpenAICompat:
		switch {
		case strings.Contains(strings.ToLower(prof.Provider.BaseURL), "openrouter"):
			return string(profile.ProviderOpenRouter)
		case strings.Contains(strings.ToLower(prof.Provider.BaseURL), "vidar:1235"):
			return string(profile.ProviderOMLX)
		default:
			return string(profile.ProviderOpenAICompat)
		}
	default:
		return string(prof.Provider.Type)
	}
}

func terminalBenchProviderSurface(prof *profile.Profile) string {
	switch prof.Provider.Type {
	case profile.ProviderOpenAICompat:
		return "openai-compat"
	case profile.ProviderAnthropic:
		return "messages"
	case profile.ProviderOpenAI:
		return "responses"
	default:
		return string(prof.Provider.Type)
	}
}

func terminalBenchHarnessWrapperName(harness string) string {
	switch harness {
	case "fiz":
		return "fiz-native"
	case "claude":
		return "claude-code"
	default:
		return harness
	}
}

func matrixSubsetID(matrix *matrixOutput, metadata map[string]any, subset *termbenchSubset) string {
	if value := metadataPathString(metadata, "benchmark.subset_id"); value != "" {
		return value
	}
	if value := metadataPathString(metadata, "provenance.benchmark_subset"); value != "" {
		return value
	}
	if matrix != nil && strings.TrimSpace(matrix.SubsetPath) != "" {
		base := filepath.Base(matrix.SubsetPath)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.TrimSpace(subset.Dataset)
}

func matrixSubsetVersion(metadata map[string]any, subset *termbenchSubset) string {
	if value := metadataPathString(metadata, "benchmark.subset_version"); value != "" {
		return value
	}
	if value := metadataPathString(metadata, "provenance.benchmark_subset_version"); value != "" {
		return value
	}
	return subset.Version
}

func terminalBenchAggregateExcludedClasses(cell matrixCell) []string {
	if len(cell.InvalidCounts) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(cell.InvalidCounts))
	for class := range cell.InvalidCounts {
		out = append(out, class)
	}
	return out
}

func terminalBenchPathFromMatrix(matrixRoot string, parts ...string) string {
	return filepath.ToSlash(filepath.Join(append([]string{matrixRoot}, parts...)...))
}

func resolveMatrixPath(workDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if workDir == "" {
		workDir = "."
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if _, err := os.Stat(filepath.Join(workDir, path)); err == nil {
		return filepath.Join(workDir, path)
	}
	return filepath.Join(workDir, path)
}

func resolveBenchmarkPath(workDir, matrixRoot, rawPath string) string {
	if rawPath == "" {
		return ""
	}
	if filepath.IsAbs(rawPath) {
		return rawPath
	}
	candidates := []string{
		filepath.Join(matrixRoot, rawPath),
		filepath.Join(workDir, rawPath),
		rawPath,
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(matrixRoot, rawPath)
}

func metadataPathString(metadata map[string]any, path string) string {
	if metadata == nil {
		return ""
	}
	var cur any = metadata
	for _, segment := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		next, ok := m[segment]
		if !ok {
			return ""
		}
		cur = next
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}

func metadataPathBool(metadata map[string]any, path string) bool {
	if metadata == nil {
		return false
	}
	var cur any = metadata
	for _, segment := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		next, ok := m[segment]
		if !ok {
			return false
		}
		cur = next
	}
	b, ok := cur.(bool)
	return ok && b
}

func metadataPathFloat(metadata map[string]any, path string) float64 {
	if metadata == nil {
		return 0
	}
	var cur any = metadata
	for _, segment := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0
		}
		next, ok := m[segment]
		if !ok {
			return 0
		}
		cur = next
	}
	switch n := cur.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func stringOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func boolOrDefault(value, fallback bool) bool {
	if value {
		return true
	}
	return fallback
}

func runDurationSeconds(run matrixRunReport) float64 {
	if run.WallSeconds != nil {
		return *run.WallSeconds
	}
	if !run.StartedAt.IsZero() && !run.FinishedAt.IsZero() {
		return run.FinishedAt.Sub(run.StartedAt).Seconds()
	}
	return 0
}

func cellScoreValue(cell matrixCell) float64 {
	if cell.NRuns == 0 {
		return 0
	}
	if cell.NValid == 0 {
		return 0
	}
	return float64(cell.NReported) / float64(cell.NValid)
}

func writeBenchmarkEvidenceJSONL(file *os.File, records []map[string]any) error {
	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(raw, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func sha256File(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
