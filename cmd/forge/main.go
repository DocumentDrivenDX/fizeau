// Command forge is a standalone CLI that wraps the forge library.
// It proves the library works end-to-end and serves as the DDx harness backend.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/forge"
	forgeConfig "github.com/DocumentDrivenDX/forge/config"
	"github.com/DocumentDrivenDX/forge/prompt"
	"github.com/DocumentDrivenDX/forge/session"
	"github.com/DocumentDrivenDX/forge/tool"
)

// Version info set via -ldflags.
var (
	Version   = "dev"
	BuildTime = ""
	GitCommit = ""
)

func main() {
	os.Exit(run())
}

func run() int {
	// Define flags
	promptFlag := flag.String("p", "", "Prompt (use @file to read from file)")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")
	providerFlag := flag.String("provider", "", "Named provider from config (e.g., vidar, openrouter)")
	model := flag.String("model", "", "Model name override")
	maxIter := flag.Int("max-iter", 0, "Max iterations")
	workDir := flag.String("work-dir", "", "Working directory")
	version := flag.Bool("version", false, "Print version")
	sysPromptFlag := flag.String("system", "", "System prompt (appended to preset)")
	presetFlag := flag.String("preset", "", "System prompt preset (forge, claude, codex, cursor, minimal)")

	flag.Parse()

	if *version {
		fmt.Printf("forge %s (commit %s, built %s)\n", Version, GitCommit, BuildTime)
		return 0
	}

	// Resolve working directory early (needed for config loading)
	wd := *workDir
	if wd == "" {
		wd, _ = os.Getwd()
	}

	// Handle subcommands
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "log":
			return cmdLog(wd, args[1:])
		case "replay":
			return cmdReplay(wd, args[1:])
		case "models":
			return cmdModels(wd, *providerFlag, args[1:])
		case "check":
			return cmdCheck(wd, *providerFlag, args[1:])
		case "providers":
			return cmdProviders(wd, *jsonOutput)
		case "version":
			fmt.Printf("forge %s (commit %s, built %s)\n", Version, GitCommit, BuildTime)
			return 0
		}
	}

	// Load config
	cfg, err := forgeConfig.Load(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 2
	}

	// Resolve prompt
	promptText, err := resolvePrompt(*promptFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 2
	}
	if promptText == "" {
		fmt.Fprintln(os.Stderr, "error: no prompt provided (use -p or pipe to stdin)")
		flag.Usage()
		return 2
	}

	// Build provider
	provName := *providerFlag
	if provName == "" {
		provName = cfg.DefaultName()
	}
	p, err := cfg.BuildProvider(provName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 2
	}

	// Apply model override
	pc, _ := cfg.GetProvider(provName)
	if *model != "" {
		pc.Model = *model
		// Rebuild with overridden model
		p, err = cfg.BuildProvider(provName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 2
		}
	}

	// Resolve max iterations
	iterations := cfg.MaxIterations
	if *maxIter > 0 {
		iterations = *maxIter
	}

	// Build tools
	tools := []forge.Tool{
		&tool.ReadTool{WorkDir: wd},
		&tool.WriteTool{WorkDir: wd},
		&tool.EditTool{WorkDir: wd},
		&tool.BashTool{WorkDir: wd},
	}

	// Build system prompt
	preset := *presetFlag
	if preset == "" {
		preset = cfg.Preset
	}
	if preset == "" {
		preset = "forge"
	}
	sysPrompt := prompt.NewFromPreset(preset).
		WithTools(tools).
		WithContextFiles(prompt.LoadContextFiles(wd)).
		WithWorkDir(wd)

	if *sysPromptFlag != "" {
		sysPrompt.WithAppend(*sysPromptFlag)
	}

	// Session logger
	sessionID := fmt.Sprintf("s-%d", os.Getpid())
	logger := session.NewLogger(cfg.SessionLogDir, sessionID)
	defer logger.Close()

	// Build request
	req := forge.Request{
		Prompt:        promptText,
		SystemPrompt:  sysPrompt.Build(),
		Provider:      p,
		Tools:         tools,
		MaxIterations: iterations,
		WorkDir:       wd,
		Callback:      logger.Callback(),
	}

	// Run with signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	result, err := forge.Run(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	// Output result
	if *jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Print(result.Output)
		if result.Output != "" && !strings.HasSuffix(result.Output, "\n") {
			fmt.Println()
		}
	}

	// Status to stderr
	fmt.Fprintf(os.Stderr, "[%s] tokens: %d in / %d out", result.Status, result.Tokens.Input, result.Tokens.Output)
	if result.CostUSD > 0 {
		fmt.Fprintf(os.Stderr, " | cost: $%.4f", result.CostUSD)
	}
	fmt.Fprintln(os.Stderr)

	switch result.Status {
	case forge.StatusSuccess:
		return 0
	default:
		return 1
	}
}

func resolvePrompt(p string) (string, error) {
	if p != "" {
		if strings.HasPrefix(p, "@") {
			data, err := os.ReadFile(p[1:])
			if err != nil {
				return "", fmt.Errorf("reading prompt file: %w", err)
			}
			return string(data), nil
		}
		return p, nil
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	return "", nil
}

func cmdProviders(workDir string, jsonOut bool) int {
	cfg, err := forgeConfig.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if jsonOut {
		data, _ := json.MarshalIndent(cfg.Providers, "", "  ")
		fmt.Println(string(data))
		return 0
	}

	defName := cfg.DefaultName()
	fmt.Printf("%-12s %-15s %-40s %-30s %s\n", "NAME", "TYPE", "URL", "MODEL", "STATUS")
	for _, name := range cfg.ProviderNames() {
		pc := cfg.Providers[name]
		status := checkProviderStatus(pc)
		marker := " "
		if name == defName {
			marker = "*"
		}
		url := pc.BaseURL
		if url == "" {
			url = "(api)"
		}
		if len(url) > 38 {
			url = url[:38] + ".."
		}
		modelStr := pc.Model
		if len(modelStr) > 28 {
			modelStr = modelStr[:28] + ".."
		}
		fmt.Printf("%s%-11s %-15s %-40s %-30s %s\n", marker, name, pc.Type, url, modelStr, status)
	}
	return 0
}

func cmdModels(workDir, providerName string, args []string) int {
	cfg, err := forgeConfig.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	showAll := len(args) > 0 && args[0] == "--all"

	if showAll {
		for _, name := range cfg.ProviderNames() {
			pc := cfg.Providers[name]
			fmt.Printf("[%s]\n", name)
			models := listModels(pc)
			if len(models) == 0 {
				fmt.Println("  (unavailable)")
			} else {
				for _, m := range models {
					fmt.Printf("  %s\n", m)
				}
			}
			fmt.Println()
		}
		return 0
	}

	name := providerName
	if name == "" {
		name = cfg.DefaultName()
	}
	pc, ok := cfg.GetProvider(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown provider %q\n", name)
		return 1
	}

	if pc.Type == "anthropic" {
		fmt.Println("Anthropic does not support model listing.")
		fmt.Printf("Configured model: %s\n", pc.Model)
		return 0
	}

	models := listModels(pc)
	for _, m := range models {
		fmt.Println(m)
	}
	return 0
}

func cmdCheck(workDir, providerName string, args []string) int {
	cfg, err := forgeConfig.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	checkAll := len(args) > 0 && args[0] == "--all"

	if checkAll {
		allOk := true
		for _, name := range cfg.ProviderNames() {
			pc := cfg.Providers[name]
			status := checkProviderStatus(pc)
			ok := strings.Contains(status, "connected") || strings.Contains(status, "configured")
			marker := "ok"
			if !ok {
				marker = "FAIL"
				allOk = false
			}
			fmt.Printf("[%s] %s: %s\n", marker, name, status)
		}
		if !allOk {
			return 1
		}
		return 0
	}

	name := providerName
	if name == "" {
		name = cfg.DefaultName()
	}
	pc, ok := cfg.GetProvider(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown provider %q\n", name)
		return 1
	}

	fmt.Printf("Provider: %s (%s)\n", name, pc.Type)
	if pc.BaseURL != "" {
		fmt.Printf("URL:      %s\n", pc.BaseURL)
	}
	status := checkProviderStatus(pc)
	fmt.Printf("Status:   %s\n", status)
	if pc.Model != "" {
		fmt.Printf("Model:    %s\n", pc.Model)
	}

	if strings.Contains(status, "unreachable") || strings.Contains(status, "no API") {
		return 1
	}
	return 0
}

func cmdLog(workDir string, args []string) int {
	cfg, err := forgeConfig.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}

	if len(args) > 0 {
		path := filepath.Join(cfg.SessionLogDir, args[0]+".jsonl")
		events, err := session.ReadEvents(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return 1
		}
		for _, e := range events {
			data, _ := json.MarshalIndent(e, "", "  ")
			fmt.Println(string(data))
		}
		return 0
	}

	entries, err := os.ReadDir(cfg.SessionLogDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			name := strings.TrimSuffix(e.Name(), ".jsonl")
			info, _ := e.Info()
			if info != nil {
				fmt.Printf("%s  %s  %d bytes\n", name, info.ModTime().Format("2006-01-02 15:04"), info.Size())
			} else {
				fmt.Println(name)
			}
		}
	}
	return 0
}

func cmdReplay(workDir string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: forge replay <session-id>")
		return 2
	}
	cfg, err := forgeConfig.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	path := filepath.Join(cfg.SessionLogDir, args[0]+".jsonl")
	if err := session.Replay(path, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 1
	}
	return 0
}

func checkProviderStatus(pc forgeConfig.ProviderConfig) string {
	if pc.Type == "anthropic" {
		if pc.APIKey == "" {
			return "no API key"
		}
		return "api key configured"
	}

	url := pc.BaseURL
	if url == "" {
		return "no URL configured"
	}
	modelsURL := strings.TrimSuffix(url, "/") + "/models"
	if strings.HasSuffix(url, "/v1") {
		modelsURL = url + "/models"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(modelsURL)
	if err != nil {
		return fmt.Sprintf("unreachable (%s)", strings.Split(err.Error(), ": ")[len(strings.Split(err.Error(), ": "))-1])
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "connected (parse error)"
	}
	return fmt.Sprintf("connected (%d models)", len(result.Data))
}

func listModels(pc forgeConfig.ProviderConfig) []string {
	if pc.Type == "anthropic" {
		return nil
	}
	url := pc.BaseURL
	if url == "" {
		return nil
	}
	modelsURL := strings.TrimSuffix(url, "/") + "/models"
	if strings.HasSuffix(url, "/v1") {
		modelsURL = url + "/models"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(modelsURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	var models []string
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models
}
