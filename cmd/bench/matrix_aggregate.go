package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type matrixCostsOutput struct {
	Cells           []matrixCostCell `json:"cells"`
	MatrixTotalUSD  float64          `json:"matrix_total_usd"`
	PerRunCapUSD    *float64         `json:"per_run_cap_usd,omitempty"`
	PerMatrixCapUSD *float64         `json:"per_matrix_cap_usd,omitempty"`
	CapDerivation   string           `json:"cap_derivation,omitempty"`
}

type matrixCostCell struct {
	Harness            string  `json:"harness"`
	ProfileID          string  `json:"profile_id"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	CachedInputTokens  int     `json:"cached_input_tokens"`
	RetriedInputTokens int     `json:"retried_input_tokens"`
	CostUSD            float64 `json:"cost_usd"`
	PricingSource      string  `json:"pricing_source,omitempty"`
}

func cmdMatrixAggregate(args []string) int {
	fs := flagSet("matrix-aggregate")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s matrix-aggregate <out>\n", benchCommandName())
		return 2
	}
	wd := resolveWorkDir(*workDir)
	outDir := fs.Arg(0)
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(wd, outDir)
	}
	runs, err := loadMatrixRunReports(outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-aggregate: %v\n", benchCommandName(), err)
		return 1
	}
	previous := readPreviousMatrixMetadata(outDir)
	output := matrixOutput{
		GeneratedAt:     time.Now().UTC(),
		SubsetPath:      previous.SubsetPath,
		Profiles:        uniqueRunStrings(runs, func(r matrixRunReport) string { return r.ProfileID }),
		Harnesses:       uniqueRunStrings(runs, func(r matrixRunReport) string { return r.Harness }),
		Reps:            maxRep(runs),
		BudgetUSD:       previous.BudgetUSD,
		PerRunBudgetUSD: previous.PerRunBudgetUSD,
		InvalidByClass:  summarizeMatrixInvalids(runs),
		InvalidRuns:     countMatrixInvalids(runs),
		Runs:            runs,
		Cells:           summarizeMatrixCells(runs),
		Notes: []string{
			"Generated from per-cell report.json files by matrix-aggregate.",
			"Null rewards are excluded from mean reward denominators and reflected in n_reported.",
			"Invalid cells are excluded from capability pass-rate denominators and listed with invalid_class.",
		},
	}
	if len(previous.Profiles) > 0 {
		output.Profiles = previous.Profiles
	}
	if len(previous.Harnesses) > 0 {
		output.Harnesses = previous.Harnesses
	}
	if previous.Reps > 0 {
		output.Reps = previous.Reps
	}

	costs := buildMatrixCosts(runs, output.PerRunBudgetUSD, output.BudgetUSD)
	if err := writeJSONAtomic(filepath.Join(outDir, "matrix.json"), output); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-aggregate: write matrix.json: %v\n", benchCommandName(), err)
		return 1
	}
	if err := writeJSONAtomic(filepath.Join(outDir, "costs.json"), costs); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-aggregate: write costs.json: %v\n", benchCommandName(), err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(outDir, "matrix.md"), []byte(renderMatrixMarkdown(output, costs)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "%s matrix-aggregate: write matrix.md: %v\n", benchCommandName(), err)
		return 1
	}
	fmt.Printf("matrix aggregate: %s\n", outDir)
	return 0
}

func loadMatrixRunReports(outDir string) ([]matrixRunReport, error) {
	var runs []matrixRunReport
	root := filepath.Join(outDir, "cells")
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != matrixReportName {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- path is found under runner-owned output dir
		if err != nil {
			return err
		}
		var report matrixRunReport
		if err := json.Unmarshal(data, &report); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		runs = append(runs, report)
		return nil
	}); err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("no %s files found under %s", matrixReportName, root)
	}
	sort.Slice(runs, func(i, j int) bool { return matrixRunKey(runs[i]) < matrixRunKey(runs[j]) })
	return runs, nil
}

func readPreviousMatrixMetadata(outDir string) matrixOutput {
	data, err := os.ReadFile(filepath.Join(outDir, "matrix.json")) // #nosec G304 -- runner-owned metadata path
	if err != nil {
		return matrixOutput{}
	}
	var out matrixOutput
	_ = json.Unmarshal(data, &out)
	return out
}

func buildMatrixCosts(runs []matrixRunReport, perRunBudgetUSD, budgetUSD float64) matrixCostsOutput {
	type acc struct {
		cell matrixCostCell
	}
	byKey := map[string]*acc{}
	var total float64
	for _, run := range runs {
		key := run.Harness + "\x00" + run.ProfileID
		a := byKey[key]
		if a == nil {
			a = &acc{cell: matrixCostCell{
				Harness:       run.Harness,
				ProfileID:     run.ProfileID,
				PricingSource: run.PricingSource,
			}}
			byKey[key] = a
		}
		a.cell.InputTokens += intValue(run.InputTokens)
		a.cell.OutputTokens += intValue(run.OutputTokens)
		a.cell.CachedInputTokens += intValue(run.CachedInputTokens)
		a.cell.RetriedInputTokens += intValue(run.RetriedInputTokens)
		a.cell.CostUSD += run.CostUSD
		total += run.CostUSD
	}
	costs := matrixCostsOutput{MatrixTotalUSD: total}
	if perRunBudgetUSD > 0 {
		costs.PerRunCapUSD = &perRunBudgetUSD
	}
	if budgetUSD > 0 {
		costs.PerMatrixCapUSD = &budgetUSD
	}
	if perRunBudgetUSD > 0 || budgetUSD > 0 {
		costs.CapDerivation = "operator-supplied caps; observation-derived values are recorded by scripts/benchmark/cost-guards when available"
	}
	for _, a := range byKey {
		costs.Cells = append(costs.Cells, a.cell)
	}
	sort.Slice(costs.Cells, func(i, j int) bool {
		if costs.Cells[i].Harness == costs.Cells[j].Harness {
			return costs.Cells[i].ProfileID < costs.Cells[j].ProfileID
		}
		return costs.Cells[i].Harness < costs.Cells[j].Harness
	})
	return costs
}

func renderMatrixMarkdown(output matrixOutput, costs matrixCostsOutput) string {
	var b strings.Builder
	b.WriteString("## Reward (mean +/- SD across N reps)\n\n")
	writeMarkdownRewardTable(&b, output)
	b.WriteString("\n## Per-task pass count (out of N reps)\n\n")
	writeMarkdownPassCountTable(&b, output)
	b.WriteString("\n## Costs\n\n")
	b.WriteString("| Cell | Input tok | Output tok | Cached tok | Retried tok | Cost ($) |\n")
	b.WriteString("|------|-----------|------------|------------|-------------|----------|\n")
	for _, cell := range costs.Cells {
		fmt.Fprintf(&b, "| %s / %s | %d | %d | %d | %d | %.6f |\n",
			cell.Harness, cell.ProfileID, cell.InputTokens, cell.OutputTokens, cell.CachedInputTokens, cell.RetriedInputTokens, cell.CostUSD)
	}
	b.WriteString("\n")
	writeMarkdownInvalidRuns(&b, output.Runs)
	b.WriteString("\n")
	writeMarkdownNonGraded(&b, output.Runs)
	return b.String()
}

func writeMarkdownRewardTable(b *strings.Builder, output matrixOutput) {
	profiles := output.Profiles
	if len(profiles) == 0 {
		profiles = uniqueRunStrings(output.Runs, func(r matrixRunReport) string { return r.ProfileID })
	}
	harnesses := output.Harnesses
	if len(harnesses) == 0 {
		harnesses = uniqueRunStrings(output.Runs, func(r matrixRunReport) string { return r.Harness })
	}
	cells := map[string]matrixCell{}
	for _, cell := range output.Cells {
		cells[cell.Harness+"\x00"+cell.ProfileID] = cell
	}
	b.WriteString("| Harness |")
	for _, profileID := range profiles {
		fmt.Fprintf(b, " %s |", profileID)
	}
	b.WriteString("\n|---------|")
	for range profiles {
		b.WriteString("---------|")
	}
	b.WriteString("\n")
	for _, harness := range harnesses {
		fmt.Fprintf(b, "| %s |", harness)
		for _, profileID := range profiles {
			cell, ok := cells[harness+"\x00"+profileID]
			if !ok || cell.MeanReward == nil || cell.SDReward == nil {
				b.WriteString(" n/a |")
				continue
			}
			fmt.Fprintf(b, " %.2f +/- %.2f (n=%d/%d) |", *cell.MeanReward, *cell.SDReward, cell.NReported, cell.NValid)
		}
		b.WriteString("\n")
	}
}

func writeMarkdownPassCountTable(b *strings.Builder, output matrixOutput) {
	tasks := uniqueRunStrings(output.Runs, func(r matrixRunReport) string { return r.TaskID })
	cells := uniqueCellLabels(output.Runs)
	passCounts := map[string]int{}
	runCounts := map[string]int{}
	for _, run := range output.Runs {
		key := run.TaskID + "\x00" + run.Harness + "\x00" + run.ProfileID
		if classifyMatrixInvalid(run) != "" {
			continue
		}
		runCounts[key]++
		if run.FinalStatus == "graded_pass" {
			passCounts[key]++
		}
	}
	b.WriteString("| Task |")
	for _, cell := range cells {
		fmt.Fprintf(b, " %s |", cell)
	}
	b.WriteString("\n|------|")
	for range cells {
		b.WriteString("---------|")
	}
	b.WriteString("\n")
	for _, task := range tasks {
		fmt.Fprintf(b, "| %s |", task)
		for _, cell := range cells {
			parts := strings.SplitN(cell, " / ", 2)
			key := task + "\x00" + parts[0] + "\x00" + parts[1]
			if runCounts[key] == 0 {
				b.WriteString(" n/a |")
				continue
			}
			fmt.Fprintf(b, " %d/%d |", passCounts[key], runCounts[key])
		}
		b.WriteString("\n")
	}
}

func writeMarkdownInvalidRuns(b *strings.Builder, runs []matrixRunReport) {
	var invalids []matrixRunReport
	for _, run := range runs {
		if class := classifyMatrixInvalid(run); class != "" {
			invalids = append(invalids, run)
		}
	}
	if len(invalids) == 0 {
		return
	}
	b.WriteString("## Invalid runs\n\n")
	b.WriteString("| Cell / rep / task | invalid_class | final_status | cause |\n")
	b.WriteString("|-------------------|---------------|--------------|-------|\n")
	for _, run := range invalids {
		cause := run.Error
		if cause == "" {
			cause = run.ProcessOutcome
		}
		fmt.Fprintf(b, "| %s / %s / %d / %s | %s | %s | %s |\n",
			run.Harness, run.ProfileID, run.Rep, run.TaskID, classifyMatrixInvalid(run), run.FinalStatus, markdownEscape(cause))
	}
}

func writeMarkdownNonGraded(b *strings.Builder, runs []matrixRunReport) {
	var nonGraded []matrixRunReport
	for _, run := range runs {
		if classifyMatrixInvalid(run) != "" {
			continue
		}
		if run.FinalStatus != "graded_pass" && run.FinalStatus != "graded_fail" {
			nonGraded = append(nonGraded, run)
		}
	}
	if len(nonGraded) == 0 {
		return
	}
	b.WriteString("## Non-graded runs\n\n")
	b.WriteString("| Cell / rep / task | final_status | cause |\n")
	b.WriteString("|-------------------|--------------|-------|\n")
	for _, run := range nonGraded {
		cause := run.Error
		if cause == "" {
			cause = run.ProcessOutcome
		}
		fmt.Fprintf(b, "| %s / %s / %d / %s | %s | %s |\n",
			run.Harness, run.ProfileID, run.Rep, run.TaskID, run.FinalStatus, markdownEscape(cause))
	}
}

func uniqueRunStrings(runs []matrixRunReport, field func(matrixRunReport) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, run := range runs {
		value := field(run)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueCellLabels(runs []matrixRunReport) []string {
	seen := map[string]bool{}
	var out []string
	for _, run := range runs {
		label := run.Harness + " / " + run.ProfileID
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func maxRep(runs []matrixRunReport) int {
	max := 0
	for _, run := range runs {
		if run.Rep > max {
			max = run.Rep
		}
	}
	return max
}

func markdownEscape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func countMatrixInvalids(runs []matrixRunReport) int {
	n := 0
	for _, run := range runs {
		if classifyMatrixInvalid(run) != "" {
			n++
		}
	}
	return n
}

func summarizeMatrixInvalids(runs []matrixRunReport) map[string]int {
	counts := map[string]int{}
	for _, run := range runs {
		if class := classifyMatrixInvalid(run); class != "" {
			counts[class]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}
