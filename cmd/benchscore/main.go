package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/agent/benchscore"
	"github.com/DocumentDrivenDX/agent/internal/productinfo"
)

func main() {
	tasksJSONL := flag.String("tasks-jsonl", "", "Path to benchmark task results JSONL")
	flag.Parse()

	if *tasksJSONL == "" {
		fmt.Fprintf(os.Stderr, "usage: %s -tasks-jsonl <path>\n", benchscoreCommandName())
		os.Exit(2)
	}

	report, err := benchscore.ScoreTaskResultsJSONL(*tasksJSONL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", benchscoreCommandName(), err)
		os.Exit(1)
	}

	data, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: marshal: %v\n", benchscoreCommandName(), err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func benchscoreCommandName() string {
	return productinfo.BinaryName + "-benchscore"
}
