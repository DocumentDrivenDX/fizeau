package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdBenchSets implements `fiz-bench bench-sets` (alias used by the shell
// driver). Subcommand-less invocation lists known bench-sets.
func cmdBenchSets(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return cmdBenchSetsList(args[1:])
		case "help", "-h", "--help":
			fmt.Fprintf(os.Stderr, "Usage: %s bench-sets [list] [--dir <path>] [--json]\n", benchCommandName())
			return 0
		}
	}
	return cmdBenchSetsList(args)
}

func cmdBenchSetsList(args []string) int {
	fs := flagSet("bench-sets list")
	dir := fs.String("dir", "", "Bench-sets directory (default: scripts/benchmark/bench-sets relative to --work-dir)")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	jsonOut := fs.Bool("json", false, "Emit JSON array instead of table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	bdir := *dir
	if bdir == "" {
		bdir = filepath.Join(resolveWorkDir(*workDir), defaultBenchSetsDir)
	}
	entries, err := os.ReadDir(bdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s bench-sets: %v\n", benchCommandName(), err)
		return 1
	}
	sets := make([]*benchSet, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		bs, err := loadBenchSet(filepath.Join(bdir, e.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s bench-sets: %v\n", benchCommandName(), err)
			return 1
		}
		sets = append(sets, bs)
	}
	sort.Slice(sets, func(i, j int) bool { return sets[i].ID < sets[j].ID })

	if *jsonOut {
		data, _ := json.MarshalIndent(sets, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	if len(sets) == 0 {
		fmt.Printf("No bench-sets found in %s.\n", bdir)
		return 0
	}
	fmt.Printf("%-32s %-16s %-22s %-8s %-6s\n", "ID", "FRAMEWORK", "DATASET", "REPS", "TASKS")
	fmt.Printf("%-32s %-16s %-22s %-8s %-6s\n", "--", "---------", "-------", "----", "-----")
	for _, bs := range sets {
		tasks := fmt.Sprintf("%d", len(bs.Tasks))
		if bs.AllTasks {
			tasks = "all"
		}
		fmt.Printf("%-32s %-16s %-22s %-8d %-6s\n", bs.ID, bs.Framework, bs.Dataset, bs.DefaultReps, tasks)
	}
	return 0
}
