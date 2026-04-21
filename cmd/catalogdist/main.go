package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DocumentDrivenDX/agent/catalogdist"
	"github.com/DocumentDrivenDX/agent/internal/productinfo"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("catalogdist", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	manifestPath := fs.String("manifest", "", "Path to catalog models.yaml")
	outDir := fs.String("out", "", "Output directory")
	channel := fs.String("channel", "stable", "Channel name")
	publishedAt := fs.String("published-at", "", "RFC3339 published timestamp (defaults to now)")
	minAgentVersion := fs.String("min-agent-version", "", fmt.Sprintf("Minimum compatible %s version", productinfo.BinaryName))
	notes := fs.String("notes", "", "Optional release notes")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}

	var ts time.Time
	var err error
	if *publishedAt != "" {
		ts, err = time.Parse(time.RFC3339, *publishedAt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid --published-at: %v\n", err)
			return 2
		}
	}

	index, err := catalogdist.Build(catalogdist.BuildOptions{
		ManifestPath:    *manifestPath,
		OutputDir:       *outDir,
		Channel:         *channel,
		PublishedAt:     ts,
		MinAgentVersion: *minAgentVersion,
		Notes:           *notes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Printf("catalog %s published to %s/%s\n", index.CatalogVersion, *outDir, *channel)
	return 0
}
