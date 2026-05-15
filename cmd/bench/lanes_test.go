package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/easel/fizeau/internal/benchmark/profile"
	"gopkg.in/yaml.v3"
)

func TestLaneClone(t *testing.T) {
	root := writeLaneCloneFixture(t)
	args := laneCloneExampleArgs(root, false)

	code := cmdLanesClone(args)
	if code != 0 {
		t.Fatalf("cmdLanesClone exit = %d", code)
	}

	gotPlan, err := loadSweepPlan(filepath.Join(root, defaultSweepPlanPath))
	if err != nil {
		t.Fatalf("load cloned sweep plan: %v", err)
	}
	wantPlan, err := loadSweepPlan(sweepPlanPath(t))
	if err != nil {
		t.Fatalf("load repo sweep plan: %v", err)
	}
	gotLane := sweepLaneMap(gotPlan)["fiz-vidar-ds4-mtp"]
	if gotLane == nil {
		t.Fatal("cloned lane fiz-vidar-ds4-mtp not found in temp sweep plan")
	}
	wantLane := sweepLaneMap(wantPlan)["fiz-vidar-ds4-mtp"]
	if wantLane == nil {
		t.Fatal("repo lane fiz-vidar-ds4-mtp not found in sweep plan")
	}
	if !reflect.DeepEqual(*gotLane, *wantLane) {
		t.Fatalf("cloned lane mismatch\n got: %+v\nwant: %+v", *gotLane, *wantLane)
	}

	for _, recipeID := range []string{"timing-baseline", "or-passing", "tb21-all"} {
		if !recipeContainsLane(gotPlan, recipeID, "fiz-vidar-ds4-mtp") {
			t.Fatalf("recipe %s does not include fiz-vidar-ds4-mtp", recipeID)
		}
	}
	for _, alias := range []string{"vidar-ds4-mtp", "ds4-mtp"} {
		if got := gotPlan.LaneAliases[alias]; got != "fiz-vidar-ds4-mtp" {
			t.Fatalf("lane alias %s = %q, want fiz-vidar-ds4-mtp", alias, got)
		}
	}

	gotProfilePath := filepath.Join(root, defaultProfilesDir, "vidar-ds4-mtp.yaml")
	gotProfile, err := profile.Load(gotProfilePath)
	if err != nil {
		t.Fatalf("load cloned profile: %v", err)
	}
	wantProfile, err := profile.Load(filepath.Join(benchRepoRoot(t), defaultProfilesDir, "vidar-ds4-mtp.yaml"))
	if err != nil {
		t.Fatalf("load repo target profile: %v", err)
	}
	gotProfile.Path = ""
	wantProfile.Path = ""
	if !reflect.DeepEqual(*gotProfile, *wantProfile) {
		t.Fatalf("cloned profile mismatch\n got: %+v\nwant: %+v", *gotProfile, *wantProfile)
	}

	readmePath := filepath.Join(benchRepoRoot(t), "scripts", "benchmark", "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	for _, want := range []string{
		"go run ./cmd/bench lanes clone",
		"--lane-id fiz-vidar-ds4-mtp",
		"--profile-id vidar-ds4-mtp",
		"--recipes timing-baseline,or-passing,tb21-all",
		"--aliases vidar-ds4-mtp,ds4-mtp",
	} {
		if !strings.Contains(string(readme), want) {
			t.Fatalf("README missing documented command fragment %q", want)
		}
	}
}

func TestLaneCloneDryRunDoesNotWrite(t *testing.T) {
	root := writeLaneCloneFixture(t)
	planPath := filepath.Join(root, defaultSweepPlanPath)
	beforePlan, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read fixture plan: %v", err)
	}

	args := laneCloneExampleArgs(root, true)
	code, output := captureStdout(t, func() int { return cmdLanesClone(args) })
	if code != 0 {
		t.Fatalf("cmdLanesClone dry-run exit = %d\noutput:\n%s", code, output)
	}

	afterPlan, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read fixture plan after dry-run: %v", err)
	}
	if string(beforePlan) != string(afterPlan) {
		t.Fatal("dry-run mutated the sweep plan")
	}
	if _, err := os.Stat(filepath.Join(root, defaultProfilesDir, "vidar-ds4-mtp.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target profile, stat err=%v", err)
	}
	for _, want := range []string{
		"Dry run: no files written.",
		filepath.Join(root, defaultSweepPlanPath),
		filepath.Join(root, defaultProfilesDir, "vidar-ds4-mtp.yaml"),
		"add aliases: vidar-ds4-mtp, ds4-mtp",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q\nfull output:\n%s", want, output)
		}
	}
}

func writeLaneCloneFixture(t *testing.T) string {
	t.Helper()
	repoRoot := benchRepoRoot(t)
	root := t.TempDir()

	srcPlan := filepath.Join(repoRoot, defaultSweepPlanPath)
	dstPlan := filepath.Join(root, defaultSweepPlanPath)
	if err := copyTextFile(srcPlan, dstPlan); err != nil {
		t.Fatalf("copy sweep plan fixture: %v", err)
	}

	profilesDir := filepath.Join(repoRoot, defaultProfilesDir)
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		t.Fatalf("read profiles dir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		src := filepath.Join(profilesDir, entry.Name())
		dst := filepath.Join(root, defaultProfilesDir, entry.Name())
		if err := copyTextFile(src, dst); err != nil {
			t.Fatalf("copy profile fixture %s: %v", entry.Name(), err)
		}
	}

	removeLaneCloneTargetFromFixture(t, dstPlan, "fiz-vidar-ds4-mtp")
	if err := os.Remove(filepath.Join(root, defaultProfilesDir, "vidar-ds4-mtp.yaml")); err != nil {
		t.Fatalf("remove target profile fixture: %v", err)
	}
	return root
}

func removeLaneCloneTargetFromFixture(t *testing.T, planPath, laneID string) {
	t.Helper()
	doc, err := loadYAMLDocument(planPath)
	if err != nil {
		t.Fatalf("load fixture plan for mutation: %v", err)
	}
	rootMap, err := yamlDocumentRootMap(doc)
	if err != nil {
		t.Fatalf("fixture plan root: %v", err)
	}
	lanesSeq, err := yamlMapSequence(rootMap, "lanes")
	if err != nil {
		t.Fatalf("fixture lanes: %v", err)
	}
	filteredLanes := make([]*yaml.Node, 0, len(lanesSeq.Content))
	for _, item := range lanesSeq.Content {
		if item.Kind == yaml.MappingNode {
			if idNode := yamlMapLookup(item, "id"); idNode != nil && idNode.Value == laneID {
				continue
			}
		}
		filteredLanes = append(filteredLanes, item)
	}
	lanesSeq.Content = filteredLanes

	recipesSeq, err := yamlMapSequence(rootMap, "recipes")
	if err != nil {
		t.Fatalf("fixture recipes: %v", err)
	}
	for _, recipeNode := range recipesSeq.Content {
		recipeLanes := yamlMapLookup(recipeNode, "lanes")
		if recipeLanes == nil || recipeLanes.Kind != yaml.SequenceNode {
			continue
		}
		filtered := make([]*yaml.Node, 0, len(recipeLanes.Content))
		for _, item := range recipeLanes.Content {
			if item.Kind == yaml.ScalarNode && item.Value == laneID {
				continue
			}
			filtered = append(filtered, item)
		}
		recipeLanes.Content = filtered
	}

	aliasMap := yamlMapLookup(rootMap, "lane_aliases")
	if aliasMap != nil && aliasMap.Kind == yaml.MappingNode {
		filtered := make([]*yaml.Node, 0, len(aliasMap.Content))
		for i := 0; i+1 < len(aliasMap.Content); i += 2 {
			keyNode := aliasMap.Content[i]
			valNode := aliasMap.Content[i+1]
			if valNode.Kind == yaml.ScalarNode && valNode.Value == laneID {
				continue
			}
			filtered = append(filtered, keyNode, valNode)
		}
		aliasMap.Content = filtered
	}

	data, err := marshalYAMLDocument(doc)
	if err != nil {
		t.Fatalf("render mutated fixture plan: %v", err)
	}
	if err := writeTextAtomic(planPath, data); err != nil {
		t.Fatalf("write mutated fixture plan: %v", err)
	}
}

func laneCloneExampleArgs(root string, dryRun bool) []string {
	args := []string{
		"--work-dir", root,
		"--from-lane", "fiz-vidar-ds4",
		"--lane-id", "fiz-vidar-ds4-mtp",
		"--profile-id", "vidar-ds4-mtp",
		"--recipes", "timing-baseline,or-passing,tb21-all",
		"--aliases", "vidar-ds4-mtp,ds4-mtp",
		"--quant-label", "ds4-native-bf16-mtp",
		"--metadata", "mtp=enabled",
		"--resolved-at", "2026-05-15",
		"--snapshot-suffix", " | mtp=enabled",
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	return args
}

func recipeContainsLane(plan *sweepPlan, recipeID, laneID string) bool {
	for _, recipe := range plan.Recipes {
		if recipe.ID != recipeID {
			continue
		}
		for _, candidate := range recipe.Lanes {
			if candidate == laneID {
				return true
			}
		}
	}
	return false
}
