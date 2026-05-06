package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/benchmark/evidence"
)

const beadBenchReportName = "report.json"

func cmdEvidenceImportBeadBench(args []string) int {
	fs := flagSet("evidence import-beadbench")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	reportPath := fs.String("report", "", "beadbench report.json file")
	outPath := fs.String("out", "", "Output JSONL path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*reportPath) == "" || strings.TrimSpace(*outPath) == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s evidence import-beadbench --report <report.json> --out <records.jsonl>\n", benchCommandName())
		return 2
	}

	wd := resolveWorkDir(*workDir)
	reportFile := resolveBenchmarkPath(wd, "", *reportPath)
	outFile := resolveBenchmarkPath(wd, "", *outPath)

	report, rawReport, err := loadBeadbenchReport(reportFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: load report: %v\n", benchCommandName(), err)
		return 1
	}

	records, err := buildBeadbenchEvidenceRecords(wd, reportFile, rawReport, report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: %v\n", benchCommandName(), err)
		return 1
	}

	if err := os.MkdirAll(filepath.Dir(outFile), 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: create output dir: %v\n", benchCommandName(), err)
		return 1
	}
	tmp, err := os.CreateTemp(filepath.Dir(outFile), filepath.Base(outFile)+".*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: create temp output: %v\n", benchCommandName(), err)
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
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: write output: %v\n", benchCommandName(), writeErr)
		return 1
	}

	validator, err := evidence.NewValidator(wd)
	if err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: %v\n", benchCommandName(), err)
		return 1
	}
	if _, err := validator.ValidateFile(tmpName); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: validate output: %v\n", benchCommandName(), err)
		return 1
	}
	if err := os.Rename(tmpName, outFile); err != nil {
		_ = os.Remove(tmpName)
		fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: rename output: %v\n", benchCommandName(), err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "%s evidence import-beadbench: imported %d record(s) to %s\n", benchCommandName(), len(records), outFile)
	return 0
}

func loadBeadbenchReport(path string) (map[string]any, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, raw, nil
}

func buildBeadbenchEvidenceRecords(workDir, reportPath string, rawReport []byte, report map[string]any) ([]map[string]any, error) {
	rawResults, ok := report["results"].([]any)
	if !ok || len(rawResults) == 0 {
		return nil, fmt.Errorf("beadbench report %s contains no results", reportPath)
	}

	armsByID := make(map[string]map[string]any)
	if rawArms, ok := report["arms"].([]any); ok {
		for _, rawArm := range rawArms {
			arm, ok := rawArm.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := arm["id"].(string); id != "" {
				armsByID[id] = arm
			}
		}
	}

	uniqueTasks := make(map[string]struct{})
	for _, rawResult := range rawResults {
		result, ok := rawResult.(map[string]any)
		if !ok {
			continue
		}
		if taskID, _ := result["task_id"].(string); taskID != "" {
			uniqueTasks[taskID] = struct{}{}
		}
	}

	records := make([]map[string]any, 0, len(rawResults))
	for _, rawResult := range rawResults {
		result, ok := rawResult.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("beadbench report %s contains a non-object result", reportPath)
		}
		record, err := buildBeadbenchEvidenceRecord(workDir, reportPath, rawReport, report, armsByID, len(uniqueTasks), result)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildBeadbenchEvidenceRecord(
	workDir, reportPath string,
	rawReport []byte,
	report map[string]any,
	armsByID map[string]map[string]any,
	nTasks int,
	result map[string]any,
) (map[string]any, error) {
	armID := bbString(result, "arm_id")
	arm := bbMap(result, "arm")
	if arm == nil {
		arm = armsByID[armID]
	}

	executeResult := bbMap(result, "execute_result")
	timeoutResult := bbMap(result, "timeout")
	verifyResult := bbMap(result, "verify")
	if executeResult == nil && timeoutResult != nil {
		executeResult = bbMap(timeoutResult, "partial_execute_result")
	}

	rawStatus := bbFirstString(
		bbString(result, "status"),
		bbString(result, "final_status"),
		bbString(result, "process_outcome"),
		bbString(executeResult, "status"),
		bbString(executeResult, "final_status"),
		bbString(executeResult, "process_outcome"),
	)
	reviewOutcome := bbFirstString(
		bbString(verifyResult, "status"),
		bbString(result, "review_outcome"),
	)
	invalidClass := classifyBeadbenchInvalid(rawStatus, result, executeResult, verifyResult, timeoutResult)
	resolvedStatus := rawStatus
	if invalidClass != "" {
		resolvedStatus = invalidClass
	}

	capturedAt := bbFirstString(
		bbString(report, "captured"),
		bbString(report, "captured_at"),
		time.Now().UTC().Format(time.RFC3339),
	)
	manifest := bbMap(report, "manifest")
	manifestVersion := bbFirstString(bbString(manifest, "version"), "unknown")
	manifestPath := bbString(report, "manifest_path")
	reportHash := sha256.Sum256(rawReport)

	record := map[string]any{
		"schema_version": evidence.SchemaVersion,
		"captured_at":    capturedAt,
		"source": map[string]any{
			"type":            "imported_report",
			"name":            "beadbench report",
			"artifact_path":   filepath.ToSlash(reportPath),
			"artifact_sha256": hex.EncodeToString(reportHash[:]),
		},
		"benchmark": map[string]any{
			"name":             "beadbench",
			"version":          manifestVersion,
			"dataset":          manifestPath,
			"scorer":           "beadbench verifier",
			"higher_is_better": true,
		},
		"subject": map[string]any{
			"model_raw": bbString(arm, "model"),
			"harness":   bbString(arm, "harness"),
			"provider":  bbFirstString(bbString(arm, "provider"), bbString(arm, "harness")),
		},
		"scope": map[string]any{
			"run_id":  bbString(result, "run_id"),
			"task_id": bbString(result, "task_id"),
			"rep":     bbInt(result, "repetition", 1),
			"n_tasks": nTasks,
		},
		"score": map[string]any{
			"metric":    "run_completion",
			"value":     bbCompletionValue(resolvedStatus),
			"raw_value": resolvedStatus,
			"n":         1,
			"passed":    bbCompletionPassed(resolvedStatus),
			"failed":    bbCompletionFailed(resolvedStatus),
		},
		"runtime":    map[string]any{},
		"provenance": map[string]any{},
		"components": map[string]any{
			"project":           bbString(result, "project"),
			"project_root":      bbString(result, "project_root"),
			"bead_id":           bbString(result, "bead_id"),
			"task_category":     bbString(result, "category"),
			"task_difficulty":   bbString(result, "difficulty"),
			"arm_id":            armID,
			"arm":               arm,
			"result":            result,
			"execute_result":    executeResult,
			"verify":            verifyResult,
			"timeout":           timeoutResult,
			"raw_status":        rawStatus,
			"review_outcome":    reviewOutcome,
			"manifest":          manifest,
			"report_manifest":   bbMap(report, "manifest"),
			"report_config":     bbMap(report, "config"),
			"planned_command":   bbAny(result, "planned_command"),
			"artifact_dir":      bbString(result, "artifact_dir"),
			"base_rev":          bbString(result, "base_rev"),
			"known_good_rev":    bbString(result, "known_good_rev"),
			"task_failure_mode": beadbenchTaskFailureMode(workDir, bbString(result, "bead_id")),
			"task_capability":   beadbenchTaskCapability(workDir, bbString(result, "bead_id")),
		},
	}
	if resolvedStatus != "" {
		record["final_status"] = resolvedStatus
	}

	if reason := bbFirstString(bbString(arm, "effort"), bbString(arm, "reasoning_control")); reason != "" {
		record["subject"].(map[string]any)["reasoning"] = reason
	}
	if resolvedStatus != "" {
		record["runtime"].(map[string]any)["outcome"] = resolvedStatus
	}
	if exitCode, ok := bbIntFromAny(result["exit_code"]); ok {
		record["runtime"].(map[string]any)["exit_code"] = exitCode
	}
	if wallSeconds, ok := bbFloatFromAny(result["duration_ms"]); ok {
		record["runtime"].(map[string]any)["wall_seconds"] = wallSeconds / 1000.0
	}
	if turns, ok := bbIntFromPaths(result, "turns", "execute_result.turns", "timeout.partial_execute_result.turns"); ok {
		record["runtime"].(map[string]any)["turns"] = turns
	}
	if toolCalls, ok := bbIntFromPaths(result, "tool_calls", "execute_result.tool_calls", "timeout.partial_execute_result.tool_calls"); ok {
		record["runtime"].(map[string]any)["tool_calls"] = toolCalls
	}
	if toolCallErrors, ok := bbIntFromPaths(result, "tool_call_errors", "execute_result.tool_call_errors", "timeout.partial_execute_result.tool_call_errors"); ok {
		record["runtime"].(map[string]any)["tool_call_errors"] = toolCallErrors
	}

	if cost, ok := bbFloatFromPaths(result, "cost_usd", "execute_result.cost_usd", "timeout.partial_execute_result.cost_usd"); ok {
		record["cost"] = map[string]any{"usd": cost}
	}

	sessionPath, sessionHash := bbStringPairFromPaths(result, "execute_result.session_log_path")
	if sessionPath == "" {
		sessionPath, sessionHash = bbStringPairFromPaths(result, "timeout.partial_execute_result.session_log_path")
	}
	if sessionPath != "" {
		record["provenance"].(map[string]any)["session_log_path"] = sessionPath
	}
	if sessionHash != "" {
		record["provenance"].(map[string]any)["session_log_sha256"] = sessionHash
	}

	trajectoryPath, trajectoryHash := bbStringPairFromPaths(result, "execute_result.trajectory_path")
	if trajectoryPath == "" {
		trajectoryPath, trajectoryHash = bbStringPairFromPaths(result, "timeout.partial_execute_result.trajectory_path")
	}
	if trajectoryPath != "" {
		record["provenance"].(map[string]any)["trajectory_path"] = trajectoryPath
	}
	if trajectoryHash != "" {
		record["provenance"].(map[string]any)["trajectory_sha256"] = trajectoryHash
	}

	if fizeauVersion := bbFirstString(
		bbString(executeResult, "fizeau_version"),
		bbString(result, "fizeau_version"),
		bbString(report, "fizeau_version"),
	); fizeauVersion != "" {
		record["provenance"].(map[string]any)["fizeau_version"] = fizeauVersion
	}
	if fizeauCommit := bbFirstString(
		bbString(executeResult, "fizeau_git_commit"),
		bbString(result, "fizeau_git_commit"),
		bbString(report, "fizeau_git_commit"),
	); fizeauCommit != "" {
		record["provenance"].(map[string]any)["fizeau_git_commit"] = fizeauCommit
	}
	if harnessWrapperName := bbFirstString(
		bbString(executeResult, "harness_wrapper_name"),
		bbString(result, "harness_wrapper_name"),
		bbString(report, "harness_wrapper_name"),
	); harnessWrapperName != "" {
		record["provenance"].(map[string]any)["harness_wrapper_name"] = harnessWrapperName
	}
	if harnessWrapperVersion := bbFirstString(
		bbString(executeResult, "harness_wrapper_version"),
		bbString(result, "harness_wrapper_version"),
		bbString(report, "harness_wrapper_version"),
	); harnessWrapperVersion != "" {
		record["provenance"].(map[string]any)["harness_wrapper_version"] = harnessWrapperVersion
	}
	if harnessCLIVersion := bbFirstString(
		bbString(executeResult, "harness_cli_version"),
		bbString(result, "harness_cli_version"),
		bbString(report, "harness_cli_version"),
	); harnessCLIVersion != "" {
		record["provenance"].(map[string]any)["harness_cli_version"] = harnessCLIVersion
	}
	if providerVersion := bbFirstString(
		bbString(executeResult, "provider_version"),
		bbString(result, "provider_version"),
		bbString(report, "provider_version"),
	); providerVersion != "" {
		record["provenance"].(map[string]any)["provider_version"] = providerVersion
	}
	if benchmarkRunnerVersion := bbFirstString(
		bbString(executeResult, "benchmark_runner_version"),
		bbString(result, "benchmark_runner_version"),
		bbString(report, "benchmark_runner_version"),
	); benchmarkRunnerVersion != "" {
		record["provenance"].(map[string]any)["benchmark_runner_version"] = benchmarkRunnerVersion
	}

	if finalClass := beadbenchFinalInvalidClass(invalidClass, rawStatus); finalClass != "" {
		record["invalid_class"] = finalClass
		record["scope"].(map[string]any)["denominator_rule"] = "exclude_invalid_runs"
		record["denominator"] = map[string]any{
			"included":         false,
			"policy":           "exclude_invalid_runs",
			"reason":           beadbenchInvalidReason(finalClass),
			"excluded_classes": []string{finalClass},
			"excluded_tasks":   []string{bbString(result, "task_id")},
			"included_count":   0,
			"excluded_count":   1,
		}
	} else {
		record["scope"].(map[string]any)["denominator_rule"] = "count_valid_runs_only"
		record["denominator"] = map[string]any{
			"included":         true,
			"policy":           "count_valid_runs_only",
			"reason":           "beadbench run reached a reportable completion state",
			"included_count":   1,
			"excluded_count":   0,
			"excluded_classes": []string{},
		}
	}

	if reviewOutcome != "" {
		record["components"].(map[string]any)["review_outcome"] = reviewOutcome
	}
	if rawStatus != "" {
		record["components"].(map[string]any)["raw_status"] = rawStatus
	}

	recordID, err := evidence.StableRecordID(record)
	if err != nil {
		return nil, err
	}
	record["record_id"] = recordID
	return record, nil
}

func beadbenchTaskCapability(workDir, beadID string) string {
	task := beadbenchCorpusEntry(workDir, beadID)
	if task == nil {
		return ""
	}
	return bbFirstString(bbString(task, "capability"), bbString(task, "failure_mode"))
}

func beadbenchTaskFailureMode(workDir, beadID string) string {
	task := beadbenchCorpusEntry(workDir, beadID)
	if task == nil {
		return ""
	}
	return bbString(task, "failure_mode")
}

func beadbenchCorpusEntry(workDir, beadID string) map[string]any {
	if beadID == "" {
		return nil
	}
	path := filepath.Join(workDir, "scripts", "beadbench", "corpus.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return beadbenchCorpusEntryFromYAML(string(raw), beadID)
}

func beadbenchCorpusEntryFromYAML(raw, beadID string) map[string]any {
	var current map[string]any
	inBeads := false
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		line = strings.Split(line, "#")[0]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !inBeads {
			if strings.TrimSuffix(strings.TrimSpace(line), ":") == "beads" {
				inBeads = true
			}
			continue
		}
		stripped := strings.TrimLeft(line, " \t")
		indent := len(line) - len(stripped)
		if strings.HasPrefix(stripped, "- ") {
			if current != nil {
				if id, _ := current["id"].(string); id == beadID {
					return current
				}
			}
			current = map[string]any{}
			stripped = strings.TrimPrefix(stripped, "- ")
		} else if indent == 0 {
			break
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(stripped, ":")
		if !ok {
			continue
		}
		current[strings.TrimSpace(key)] = strings.TrimSpace(strings.Trim(value, `"'`))
	}
	if current != nil {
		if id, _ := current["id"].(string); id == beadID {
			return current
		}
	}
	return nil
}

func beadbenchFinalInvalidClass(invalidClass, rawStatus string) string {
	if invalidClass != "" {
		return invalidClass
	}
	switch rawStatus {
	case matrixInvalidQuota, matrixInvalidAuth, matrixInvalidSetup, matrixInvalidProvider:
		return rawStatus
	default:
		return ""
	}
}

func beadbenchInvalidReason(class string) string {
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

func classifyBeadbenchInvalid(rawStatus string, result, executeResult, verifyResult, timeoutResult map[string]any) string {
	switch rawStatus {
	case matrixInvalidQuota, matrixInvalidAuth, matrixInvalidSetup, matrixInvalidProvider:
		return rawStatus
	case "auth_fail":
		return matrixInvalidAuth
	case "install_fail_permanent", "install_failed":
		return matrixInvalidSetup
	case "provider_refusal":
		return matrixInvalidProvider
	case "harness_crash", "runner_error":
		// Fall through to signal matching.
	}

	blob := strings.ToLower(strings.Join(filterNonEmptyStrings([]string{
		bbString(result, "error"),
		bbString(result, "message"),
		bbString(result, "status"),
		bbString(executeResult, "error"),
		bbString(executeResult, "message"),
		bbString(executeResult, "status"),
		bbString(timeoutResult, "error"),
		bbString(timeoutResult, "progress_class"),
		bbString(verifyResult, "reason"),
		bbString(verifyResult, "status"),
	}), "\n"))

	switch {
	case matrixInvalidQuotaPattern.MatchString(blob):
		return matrixInvalidQuota
	case matrixInvalidAuthPattern.MatchString(blob):
		return matrixInvalidAuth
	case matrixInvalidSetupPattern.MatchString(blob):
		return matrixInvalidSetup
	case matrixInvalidProviderPattern.MatchString(blob):
		return matrixInvalidProvider
	default:
		return ""
	}
}

func bbCompletionValue(status string) float64 {
	if bbCompletionPassed(status) == 1 {
		return 1
	}
	return 0
}

func bbCompletionPassed(status string) int {
	if status == "" {
		return 0
	}
	if beadbenchFinalInvalidClass("", status) != "" {
		return 0
	}
	switch status {
	case "timeout", "runner_error", "dry_run", "harness_crash", "auth_fail", "provider_refusal", "install_fail_permanent", "install_failed":
		return 0
	}
	return 1
}

func bbCompletionFailed(status string) int {
	if bbCompletionPassed(status) == 1 {
		return 0
	}
	return 1
}

func bbFirstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func bbString(doc map[string]any, key string) string {
	if doc == nil {
		return ""
	}
	if value, ok := doc[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func bbAny(doc map[string]any, key string) any {
	if doc == nil {
		return nil
	}
	return doc[key]
}

func bbMap(doc map[string]any, key string) map[string]any {
	if doc == nil {
		return nil
	}
	value, ok := doc[key]
	if !ok {
		return nil
	}
	child, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return child
}

func bbInt(doc map[string]any, key string, fallback int) int {
	if doc == nil {
		return fallback
	}
	value, ok := doc[key]
	if !ok {
		return fallback
	}
	if out, ok := bbIntFromAny(value); ok {
		return out
	}
	return fallback
}

func bbIntFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func bbFloatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func bbIntFromPaths(doc map[string]any, paths ...string) (int, bool) {
	for _, path := range paths {
		if value, ok := bbLookupPath(doc, path); ok {
			if out, ok := bbIntFromAny(value); ok {
				return out, true
			}
		}
	}
	return 0, false
}

func bbFloatFromPaths(doc map[string]any, paths ...string) (float64, bool) {
	for _, path := range paths {
		if value, ok := bbLookupPath(doc, path); ok {
			if out, ok := bbFloatFromAny(value); ok {
				return out, true
			}
		}
	}
	return 0, false
}

func bbStringPairFromPaths(doc map[string]any, path string) (string, string) {
	value, ok := bbLookupPath(doc, path)
	if !ok {
		return "", ""
	}
	if child, ok := value.(map[string]any); ok {
		return bbString(child, "path"), bbString(child, "sha256")
	}
	pathValue, _ := value.(string)
	if pathValue == "" {
		return "", ""
	}
	hashPath := path
	if strings.HasSuffix(hashPath, "_path") {
		hashPath = strings.TrimSuffix(hashPath, "_path") + "_sha256"
	}
	if hashValue, ok := bbLookupPath(doc, hashPath); ok {
		if hash, ok := hashValue.(string); ok {
			return pathValue, hash
		}
	}
	return pathValue, ""
}

func bbLookupPath(doc map[string]any, path string) (any, bool) {
	if doc == nil {
		return nil, false
	}
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

func filterNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
