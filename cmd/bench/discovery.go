package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	fizeau "github.com/easel/fizeau"
	agentConfig "github.com/easel/fizeau/internal/config"
)

// Candidate is one (harness, provider, model) triple discovered from config.
type Candidate struct {
	Harness   string `json:"harness"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	CostClass string `json:"cost_class,omitempty"`
	Available bool   `json:"available"`
}

// discoverCandidates enumerates available harnesses and, for provider-backed
// harnesses, the configured models. It creates a FizeauService via fizeau.New with
// the loaded ServiceConfig and calls ListHarnesses + ListModels per
// CONTRACT-003.
func discoverCandidates(wd string) ([]Candidate, error) {
	cfg, err := agentConfig.Load(wd)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	svc, err := fizeau.New(fizeau.ServiceOptions{
		ServiceConfig: &configAdapter{cfg: cfg, workDir: wd},
	})
	if err != nil {
		return nil, fmt.Errorf("new service: %w", err)
	}

	ctx := context.Background()

	harnesses, err := svc.ListHarnesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list harnesses: %w", err)
	}

	// Build one candidate per available subprocess-style harness (claude, codex, etc.).
	var candidates []Candidate
	for _, h := range harnesses {
		if !h.Available {
			continue
		}
		// "subprocess" harnesses are billed through subscription or local binary;
		// add a single candidate with no explicit model.
		if h.Type == "subprocess" {
			candidates = append(candidates, Candidate{
				Harness:   h.Name,
				CostClass: h.CostClass,
				Available: true,
			})
			continue
		}
		// "native" harnesses are provider-backed; enumerate per-provider models.
		models, err := svc.ListModels(ctx, fizeau.ModelFilter{
			Harness: h.Name,
		})
		if err != nil || len(models) == 0 {
			// Surface the harness even if model listing fails.
			candidates = append(candidates, Candidate{
				Harness:   h.Name,
				CostClass: h.CostClass,
				Available: true,
			})
			continue
		}
		for _, m := range models {
			candidates = append(candidates, Candidate{
				Harness:   h.Name,
				Provider:  m.Provider,
				Model:     m.ID,
				CostClass: h.CostClass,
				Available: m.Available,
			})
		}
	}

	return candidates, nil
}

// cmdDiscover implements the 'discover' subcommand.
func cmdDiscover(args []string) int {
	fs := flagSet("discover")
	jsonOut := fs.Bool("json", false, "Emit JSON array instead of table")
	workDir := fs.String("work-dir", "", "Agent working directory (default: cwd)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	wd := resolveWorkDir(*workDir)
	candidates, err := discoverCandidates(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s discover: %v\n", benchCommandName(), err)
		return 1
	}

	if *jsonOut {
		data, _ := json.MarshalIndent(candidates, "", "  ")
		fmt.Println(string(data))
		return 0
	}

	// Tabular output.
	if len(candidates) == 0 {
		fmt.Println("No candidates discovered. Check agent config.")
		return 0
	}
	fmt.Printf("%-16s %-20s %-40s %-10s\n", "HARNESS", "PROVIDER", "MODEL", "COST_CLASS")
	fmt.Printf("%-16s %-20s %-40s %-10s\n", "-------", "--------", "-----", "----------")
	for _, c := range candidates {
		fmt.Printf("%-16s %-20s %-40s %-10s\n", c.Harness, c.Provider, c.Model, c.CostClass)
	}
	return 0
}
