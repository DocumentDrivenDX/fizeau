package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/agent/internal/renamecheck"
)

func main() {
	root := flag.String("repo", ".", "repository root to scan")
	fail := flag.Bool("fail", false, "exit non-zero when unallowlisted hits are found")
	flag.Parse()

	findings, err := renamecheck.Run(renamecheck.Options{Root: *root, Out: os.Stdout})
	if err != nil {
		fmt.Fprintf(os.Stderr, "rename-noise: %v\n", err)
		os.Exit(2)
	}
	if *fail && len(findings) > 0 {
		os.Exit(1)
	}
}
