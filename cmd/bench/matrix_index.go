package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/easel/fizeau/internal/benchmark/profile"
	"github.com/easel/fizeau/internal/fiztools"
)

type matrixIndexRow struct {
	Dataset         string  `json:"dataset"`
	TaskID          string  `json:"task_id"`
	Provider        string  `json:"provider"`
	Model           string  `json:"model"`
	Harness         string  `json:"harness"`
	ProfileID       string  `json:"profile_id"`
	ProfilePath     string  `json:"profile_path,omitempty"`
	ProfileSnap     string  `json:"profile_snapshot,omitempty"`
	OriginalRep     int     `json:"original_rep"`
	RunIndex        int     `json:"run_index"`
	FinalStatus     string  `json:"final_status"`
	Reward          *int    `json:"reward,omitempty"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	CostUSD         float64 `json:"cost_usd"`
	WallSeconds     float64 `json:"wall_seconds"`
	StartedAt       string  `json:"started_at,omitempty"`
	FinishedAt      string  `json:"finished_at,omitempty"`
	FizToolsVersion int     `json:"fiz_tools_version"`
	SourcePath      string  `json:"source_path"`
	CanonicalPath   string  `json:"canonical_path,omitempty"`
}

type matrixIndexSummaryRow struct {
	Dataset      string
	Provider     string
	Model        string
	Harness      string
	ProfileID    string
	Reports      int
	Pass         int
	GradedFail   int
	Invalid      int
	Crash        int
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	WallSeconds  float64
}

type profileProviderInfo struct {
	Provider string
	Model    string
	Metadata profile.Metadata
}

func cmdMatrixIndex(args []string) int {
	fs := flagSet("matrix-index")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	root := fs.String("root", "benchmark-results", "Root to scan for report.json files")
	out := fs.String("out", "", "Directory for indexes (default: <canonical-out>/indexes or <root>/indexes)")
	canonicalOut := fs.String("canonical-out", "", "Optional canonical output root; indexes go under <canonical-out>/indexes by default")
	copyCells := fs.Bool("copy", false, "Copy each source cell directory into canonical cells/")
	// --fiz-tools-version is the canonical flag; --fiz-version is kept as a
	// backward-compatible alias for shell wrappers / tooling that hasn't
	// migrated yet.
	fizVersion := fs.Int("fiz-tools-version", fiztools.Version, "Fiz tools version (agent-behavior identity) to stamp on historical rows missing explicit provenance")
	fizVersionLegacy := fs.String("fiz-version", "", "DEPRECATED alias for --fiz-tools-version; if set, expected to be \"v<N>\" or \"<N>\"")
	dataset := fs.String("dataset", "terminal-bench-2-1", "Dataset label to attach to indexed rows")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	wd := resolveWorkDir(*workDir)
	scanRoot := resolveMaybeAbs(wd, *root)
	canonicalRoot := ""
	if *canonicalOut != "" {
		canonicalRoot = resolveMaybeAbs(wd, *canonicalOut)
	}
	outDir := *out
	if outDir == "" {
		if canonicalRoot != "" {
			outDir = filepath.Join(canonicalRoot, "indexes")
		} else {
			outDir = filepath.Join(scanRoot, "indexes")
		}
	} else {
		outDir = resolveMaybeAbs(wd, outDir)
	}

	profilesDir := filepath.Join(wd, defaultProfilesDir)
	profiles, _ := loadMatrixIndexProfiles(profilesDir)
	if canonicalRoot != "" {
		// Snapshot the profile catalog into <canonical>/profiles/. This is the
		// point-in-time source of truth for what each profile_id meant when
		// these cells were collected; reporting tools join on profile_id to
		// project arbitrary dimensions (server, quant_label, runtime, etc.).
		if n, err := snapshotProfileCatalog(canonicalRoot, profilesDir); err != nil {
			fmt.Fprintf(os.Stderr, "%s matrix-index: snapshot profiles: %v\n", benchCommandName(), err)
			return 1
		} else {
			fmt.Fprintf(os.Stderr, "matrix-index: snapshotted %d profiles to %s/profiles/\n", n, canonicalRoot)
		}
	}
	// Honour the deprecated --fiz-version alias when set; accepts "v<N>" or
	// "<N>" and falls back to the explicit --fiz-tools-version value.
	effectiveToolsVersion := *fizVersion
	if legacy := strings.TrimSpace(strings.TrimPrefix(*fizVersionLegacy, "v")); legacy != "" {
		if n, err := strconv.Atoi(legacy); err == nil {
			effectiveToolsVersion = n
		}
	}
	rows, err := collectMatrixIndexRows(scanRoot, canonicalRoot, *copyCells, effectiveToolsVersion, *dataset, profiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-index: %v\n", benchCommandName(), err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "%s matrix-index: no matrix report.json files found under %s\n", benchCommandName(), scanRoot)
		return 1
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-index: mkdir %s: %v\n", benchCommandName(), outDir, err)
		return 1
	}
	if err := writeMatrixIndexJSONL(filepath.Join(outDir, "runs.jsonl"), rows); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-index: write runs.jsonl: %v\n", benchCommandName(), err)
		return 1
	}
	summary := summarizeMatrixIndexRows(rows)
	if err := writeMatrixIndexCSV(filepath.Join(outDir, "summary.csv"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-index: write summary.csv: %v\n", benchCommandName(), err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(outDir, "summary.md"), []byte(renderMatrixIndexMarkdown(summary)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-index: write summary.md: %v\n", benchCommandName(), err)
		return 1
	}
	fmt.Printf("matrix index: %d reports -> %s\n", len(rows), outDir)
	return 0
}

func resolveMaybeAbs(wd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(wd, path)
}

func loadMatrixIndexProfiles(dir string) (map[string]profileProviderInfo, error) {
	out := map[string]profileProviderInfo{}
	profiles, err := profile.LoadDir(dir)
	if err != nil {
		return out, err
	}
	for _, p := range profiles {
		out[p.ID] = profileProviderInfo{
			Provider: string(p.Provider.Type),
			Model:    p.Provider.Model,
			Metadata: p.Metadata,
		}
	}
	return out, nil
}

func collectMatrixIndexRows(root, canonicalRoot string, copyCells bool, fallbackVersion int, dataset string, profiles map[string]profileProviderInfo) ([]matrixIndexRow, error) {
	var rows []matrixIndexRow
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if canonicalRoot != "" && path != canonicalRoot {
			if rel, relErr := filepath.Rel(canonicalRoot, path); relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() || d.Name() != matrixReportName {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/indexes/") {
			return nil
		}
		raw, err := os.ReadFile(path) // #nosec G304 G122 -- path discovered under operator-supplied benchmark-results root; WalkDir TOCTOU acceptable for index build
		if err != nil {
			return err
		}
		var report matrixRunReport
		if err := json.Unmarshal(raw, &report); err != nil {
			return nil
		}
		if report.TaskID == "" || report.ProfileID == "" || report.Harness == "" {
			return nil
		}
		row := matrixIndexRowFromReport(path, report, fallbackVersion, dataset, profiles)
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(rows, func(i, j int) bool {
		return matrixIndexSortKey(rows[i]) < matrixIndexSortKey(rows[j])
	})
	assignMatrixIndexRunIndexes(rows)
	if canonicalRoot != "" {
		for i := range rows {
			rows[i].CanonicalPath = matrixIndexCanonicalReportPath(canonicalRoot, rows[i])
			if copyCells {
				if err := copyMatrixIndexCell(rows[i].SourcePath, rows[i].CanonicalPath); err != nil {
					return nil, err
				}
				// Stamp fiz_tools_version on historical reports that predate
				// the field. Other dimensions (server, model_family, quant,
				// runtime) are intentionally NOT stamped here — they live in
				// the snapshotted profile catalog at <canonical>/profiles/
				// and are joined on profile_id at query/index time.
				if err := backfillFizToolsVersion(rows[i].CanonicalPath); err != nil {
					return nil, err
				}
			}
		}
	}
	return rows, nil
}

// backfillFizToolsVersion stamps fiz_tools_version=1 on historical canonical
// reports that predate the field. Bump the constant in internal/fiztools when
// agent behavior changes; do NOT retroactively bump historical cells.
func backfillFizToolsVersion(reportPath string) error {
	raw, err := os.ReadFile(reportPath) // #nosec G304 -- canonical path under operator-supplied root
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	if v, ok := doc["fiz_tools_version"]; ok && v != nil {
		if f, isFloat := v.(float64); isFloat && f > 0 {
			return nil
		}
	}
	doc["fiz_tools_version"] = 1
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(reportPath, out, 0o600)
}

// snapshotProfileCatalog copies all profile YAMLs into <canonical>/profiles/.
// This is the point-in-time catalog that aggregation/reporting tools join
// against on profile_id, so the canonical cells tree is self-contained and
// independent of subsequent mutations to scripts/benchmark/profiles/.
func snapshotProfileCatalog(canonicalRoot, sourceProfilesDir string) (int, error) {
	dest := filepath.Join(canonicalRoot, "profiles")
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(sourceProfilesDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		src := filepath.Join(sourceProfilesDir, e.Name())
		dst := filepath.Join(dest, e.Name())
		raw, err := os.ReadFile(src) // #nosec G304 -- source is operator-controlled profiles dir
		if err != nil {
			return count, err
		}
		if err := os.WriteFile(dst, raw, 0o600); err != nil { // #nosec G304 G703 -- dst joins operator-controlled dest dir with sanitized filename from same source dir
			return count, err
		}
		count++
	}
	return count, nil
}

func matrixIndexRowFromReport(path string, report matrixRunReport, fallbackVersion int, dataset string, profiles map[string]profileProviderInfo) matrixIndexRow {
	provider := ""
	model := ""
	if info, ok := profiles[report.ProfileID]; ok {
		provider = info.Provider
		model = info.Model
	}
	if provider == "" || model == "" {
		provider, model = inferProviderModelFromProfileID(report.ProfileID)
	}
	// Prefer the version stamped on the report (post-schema-change runs);
	// fall back to the operator-supplied label only when the report doesn't
	// carry one (legacy migrated rows).
	toolsVersion := report.FizToolsVersion
	if toolsVersion == 0 {
		toolsVersion = fallbackVersion
	}
	return matrixIndexRow{
		Dataset:         dataset,
		TaskID:          report.TaskID,
		Provider:        provider,
		Model:           model,
		Harness:         effectiveMatrixIndexHarness(report),
		ProfileID:       report.ProfileID,
		ProfilePath:     report.ProfilePath,
		ProfileSnap:     report.ProfileSnapshot,
		OriginalRep:     report.Rep,
		FinalStatus:     report.FinalStatus,
		Reward:          report.Reward,
		InputTokens:     intValue(report.InputTokens),
		OutputTokens:    intValue(report.OutputTokens),
		CostUSD:         report.CostUSD,
		WallSeconds:     floatValue(report.WallSeconds),
		StartedAt:       formatMatrixIndexTime(report.StartedAt),
		FinishedAt:      formatMatrixIndexTime(report.FinishedAt),
		FizToolsVersion: toolsVersion,
		SourcePath:      path,
	}
}

func effectiveMatrixIndexHarness(report matrixRunReport) string {
	switch {
	case strings.Contains(report.ProfileID, "fiz-harness-codex"):
		return "codex"
	case strings.Contains(report.ProfileID, "fiz-harness-claude"):
		return "claude"
	case strings.Contains(report.ProfileID, "fiz-harness-opencode"):
		return "opencode"
	case strings.Contains(report.ProfileID, "fiz-harness-pi"):
		return "pi"
	default:
		return report.Harness
	}
}

func inferProviderModelFromProfileID(profileID string) (string, string) {
	switch {
	case strings.Contains(profileID, "openrouter"):
		return "openrouter", profileID
	case strings.Contains(profileID, "openai"):
		return "openai", profileID
	case strings.Contains(profileID, "sindri") || strings.Contains(profileID, "bragi"):
		return "vllm", profileID
	case strings.Contains(profileID, "vidar"):
		return "omlx", profileID
	case strings.Contains(profileID, "grendel"):
		return "rapid-mlx", profileID
	default:
		return "unknown", profileID
	}
}

func formatMatrixIndexTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func matrixIndexSortKey(row matrixIndexRow) string {
	return strings.Join([]string{
		row.Dataset,
		row.TaskID,
		row.Provider,
		row.Model,
		row.Harness,
		row.ProfileID,
		row.StartedAt,
		row.FinishedAt,
		row.SourcePath,
	}, "\x00")
}

func assignMatrixIndexRunIndexes(rows []matrixIndexRow) {
	counts := map[string]int{}
	for i := range rows {
		key := strings.Join([]string{
			rows[i].Dataset,
			rows[i].TaskID,
			rows[i].Provider,
			rows[i].Model,
			rows[i].Harness,
		}, "\x00")
		counts[key]++
		rows[i].RunIndex = counts[key]
	}
}

func matrixIndexCanonicalReportPath(root string, row matrixIndexRow) string {
	// Canonical layout: <root>/cells/<dataset>/<task>/<profile_id>/rep-NNN/report.json
	// profile_id is the primary key; per-cell projection dimensions
	// (server, model_family, quant_label, runtime, fiz_tools_version) are
	// stamped on report.json for index-time grouping.
	return filepath.Join(root, "cells", row.Dataset, slugPath(row.TaskID), slugPath(row.ProfileID), fmt.Sprintf("rep-%03d", row.RunIndex), matrixReportName)
}

func slugPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "\t", "-")
	value = replacer.Replace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			r = '-'
		}
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func copyMatrixIndexCell(sourceReport, destReport string) error {
	sourceDir := filepath.Dir(sourceReport)
	destDir := filepath.Dir(destReport)
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}
	return copyDirContents(sourceDir, destDir)
}

func copyDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src) // #nosec G304 -- source discovered under operator-supplied benchmark-results root
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm) // #nosec G304 -- dst is operator-supplied output path
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func writeMatrixIndexJSONL(path string, rows []matrixIndexRow) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- path is operator-supplied output path
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

func summarizeMatrixIndexRows(rows []matrixIndexRow) []matrixIndexSummaryRow {
	byKey := map[string]*matrixIndexSummaryRow{}
	for _, row := range rows {
		key := strings.Join([]string{row.Dataset, row.Provider, row.Model, row.Harness, row.ProfileID}, "\x00")
		s := byKey[key]
		if s == nil {
			s = &matrixIndexSummaryRow{Dataset: row.Dataset, Provider: row.Provider, Model: row.Model, Harness: row.Harness, ProfileID: row.ProfileID}
			byKey[key] = s
		}
		s.Reports++
		if row.Reward != nil && *row.Reward == 1 {
			s.Pass++
		} else if row.Reward != nil && *row.Reward == 0 && strings.HasPrefix(row.FinalStatus, "graded_") {
			s.GradedFail++
		}
		if strings.HasPrefix(row.FinalStatus, "invalid_") {
			s.Invalid++
		}
		if row.FinalStatus == "harness_crash" {
			s.Crash++
		}
		s.InputTokens += row.InputTokens
		s.OutputTokens += row.OutputTokens
		s.CostUSD += row.CostUSD
		s.WallSeconds += row.WallSeconds
	}
	out := make([]matrixIndexSummaryRow, 0, len(byKey))
	for _, row := range byKey {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.Join([]string{out[i].Dataset, out[i].Provider, out[i].Model, out[i].Harness, out[i].ProfileID}, "\x00") <
			strings.Join([]string{out[j].Dataset, out[j].Provider, out[j].Model, out[j].Harness, out[j].ProfileID}, "\x00")
	})
	return out
}

func writeMatrixIndexCSV(path string, rows []matrixIndexSummaryRow) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- path is operator-supplied output path
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"dataset", "provider", "model", "harness", "profile_id", "reports", "pass", "graded_fail", "invalid", "crash", "input_tokens", "output_tokens", "cost_usd", "wall_seconds"}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write([]string{
			row.Dataset,
			row.Provider,
			row.Model,
			row.Harness,
			row.ProfileID,
			fmt.Sprintf("%d", row.Reports),
			fmt.Sprintf("%d", row.Pass),
			fmt.Sprintf("%d", row.GradedFail),
			fmt.Sprintf("%d", row.Invalid),
			fmt.Sprintf("%d", row.Crash),
			fmt.Sprintf("%d", row.InputTokens),
			fmt.Sprintf("%d", row.OutputTokens),
			fmt.Sprintf("%.6f", row.CostUSD),
			fmt.Sprintf("%.3f", row.WallSeconds),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func renderMatrixIndexMarkdown(rows []matrixIndexSummaryRow) string {
	var b strings.Builder
	b.WriteString("| Dataset | Provider | Model | Harness | Profile | Reports | Pass | Fail | Invalid | Crash | Cost ($) |\n")
	b.WriteString("|---|---|---|---|---|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %d | %d | %d | %d | %.4f |\n",
			row.Dataset, row.Provider, row.Model, row.Harness, row.ProfileID, row.Reports, row.Pass, row.GradedFail, row.Invalid, row.Crash, row.CostUSD)
	}
	return b.String()
}

func floatValue(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
