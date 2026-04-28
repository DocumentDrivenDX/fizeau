package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/corpus"
)

// corpusRoot is the conventional location for the curated benchmark corpus.
// It is intentionally pinned to the repo (workDir) under scripts/beadbench
// so the corpus travels with the agent codebase, not with end-user state.
const corpusRoot = "scripts/beadbench"

func cmdCorpus(workDir string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ddx-agent corpus <promote|validate|list> [flags]")
		return 2
	}
	switch args[0] {
	case "promote":
		return cmdCorpusPromote(workDir, args[1:])
	case "validate":
		return cmdCorpusValidate(workDir, args[1:])
	case "list":
		return cmdCorpusList(workDir, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown corpus subcommand %q\n", args[0])
		return 2
	}
}

func cmdCorpusValidate(workDir string, args []string) int {
	fs := flag.NewFlagSet("corpus validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root := filepath.Join(workDir, corpusRoot)
	if err := corpus.Validate(root); err != nil {
		fmt.Fprintf(os.Stderr, "corpus invalid:\n%s\n", err)
		return 1
	}
	fmt.Println("corpus: ok")
	return 0
}

func cmdCorpusList(workDir string, args []string) int {
	fs := flag.NewFlagSet("corpus list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "Emit JSON instead of a human-readable table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root := filepath.Join(workDir, corpusRoot)
	loaded, err := corpus.Load(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if *jsonOut {
		out, _ := json.MarshalIndent(loaded.Index, "", "  ")
		fmt.Println(string(out))
		return 0
	}
	fmt.Printf("%-22s %-8s %-32s %s\n", "BEAD", "KIND", "TAG", "PROMOTED")
	for _, e := range loaded.Index.Beads {
		kind := "capability"
		tag := e.Capability
		if e.FailureMode != "" {
			kind = "failure_mode"
			tag = e.FailureMode
		}
		fmt.Printf("%-22s %-8s %-32s %s by %s\n", e.BeadID, kind, tag, e.Promoted, e.PromotedBy)
	}
	return 0
}

func cmdCorpusPromote(workDir string, args []string) int {
	fs := flag.NewFlagSet("corpus promote", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	capability := fs.String("capability", "", "Capability tag (must be in capabilities.yaml)")
	failureMode := fs.String("failure-mode", "", "Failure-mode tag (must be in capabilities.yaml)")
	difficulty := fs.String("difficulty", "", "easy|medium|hard")
	promptKind := fs.String("prompt-kind", "", "implement-with-spec|port-by-analogy|debug-and-fix|review")
	notes := fs.String("notes", "", "Free-text notes describing why the bead is instructive")
	baseRev := fs.String("base-rev", "", "Pre-change revision (defaults to bead.parent_commit_sha)")
	knownGoodRev := fs.String("known-good-rev", "", "Resolution revision (defaults to bead.closing_commit_sha)")
	projectRoot := fs.String("project-root", "", "Repository root for the bead (defaults to workDir)")
	promotedBy := fs.String("promoted-by", "", "Identity recording the promotion (defaults to $USER)")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt")

	// Accept the bead-id either before or after flags. We pull the first
	// non-flag positional out of args, then let the standard flag parser
	// handle the rest.
	beadID, parseArgs := extractFirstPositional(args)
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	if beadID == "" {
		fmt.Fprintln(os.Stderr, "usage: ddx-agent corpus promote <bead-id> [flags]")
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected positional argument(s): %v\n", fs.Args())
		return 2
	}

	root := filepath.Join(workDir, corpusRoot)

	loaded, err := corpus.Load(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load corpus: %v\n", err)
		return 1
	}
	if loaded.HasBead(beadID) {
		fmt.Fprintf(os.Stderr, "error: bead %q is already in corpus.yaml\n", beadID)
		return 1
	}

	bead, err := loadBeadFromJSONL(filepath.Join(workDir, ".ddx", "beads.jsonl"), beadID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if !strings.EqualFold(bead.Status, "closed") {
		fmt.Fprintf(os.Stderr, "error: bead %q is not closed (status=%q); promotion only accepts closed beads\n", beadID, bead.Status)
		return 1
	}

	resolvedBase := strings.TrimSpace(*baseRev)
	if resolvedBase == "" {
		resolvedBase = bead.ParentCommitSHA
	}
	resolvedGood := strings.TrimSpace(*knownGoodRev)
	if resolvedGood == "" {
		resolvedGood = bead.ClosingCommitSHA
	}
	if resolvedGood == "" {
		fmt.Fprintf(os.Stderr,
			"error: bead %q has no closing_commit_sha and no --known-good-rev was provided\n", beadID)
		return 1
	}
	if resolvedBase == "" {
		fmt.Fprintf(os.Stderr,
			"error: bead %q has no parent_commit_sha and no --base-rev was provided\n", beadID)
		return 1
	}

	pr := strings.TrimSpace(*projectRoot)
	if pr == "" {
		pr = workDir
	}
	by := strings.TrimSpace(*promotedBy)
	if by == "" {
		by = os.Getenv("USER")
	}
	if by == "" {
		by = "unknown"
	}

	req := corpus.PromoteRequest{
		BeadID:       beadID,
		ProjectRoot:  pr,
		BaseRev:      resolvedBase,
		KnownGoodRev: resolvedGood,
		Captured:     time.Now().UTC().Format("2006-01-02"),
		PromotedBy:   by,
		Capability:   strings.TrimSpace(*capability),
		FailureMode:  strings.TrimSpace(*failureMode),
		Difficulty:   strings.TrimSpace(*difficulty),
		PromptKind:   strings.TrimSpace(*promptKind),
		Notes:        *notes,
	}

	// Show the plan before mutating anything; gate on --yes.
	fmt.Println("about to promote:")
	fmt.Printf("  bead_id:        %s\n", req.BeadID)
	fmt.Printf("  project_root:   %s\n", req.ProjectRoot)
	fmt.Printf("  base_rev:       %s\n", req.BaseRev)
	fmt.Printf("  known_good_rev: %s\n", req.KnownGoodRev)
	if req.Capability != "" {
		fmt.Printf("  capability:     %s\n", req.Capability)
	}
	if req.FailureMode != "" {
		fmt.Printf("  failure_mode:   %s\n", req.FailureMode)
	}
	fmt.Printf("  difficulty:     %s\n", req.Difficulty)
	fmt.Printf("  prompt_kind:    %s\n", req.PromptKind)
	fmt.Printf("  notes:          %s\n", req.Notes)
	fmt.Printf("  promoted_by:    %s\n", req.PromotedBy)

	if !*yes {
		fmt.Print("proceed? (y/N) ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "y") {
			fmt.Fprintln(os.Stderr, "aborted")
			return 1
		}
	}

	if err := corpus.Promote(root, req); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("promoted %s\n", req.BeadID)
	return 0
}

// extractFirstPositional returns the first argument that does not begin
// with "-" along with the remaining args (with that positional removed).
// This lets the user write `corpus promote <bead-id> --flag value` even
// though the standard flag package stops at the first non-flag argument.
func extractFirstPositional(args []string) (string, []string) {
	for i, a := range args {
		if a == "--" {
			// Everything after -- is positional; first one is our bead id.
			if i+1 < len(args) {
				rest := append([]string{}, args[:i]...)
				rest = append(rest, args[i+2:]...)
				return args[i+1], rest
			}
			return "", args
		}
		if !strings.HasPrefix(a, "-") {
			rest := append([]string{}, args[:i]...)
			rest = append(rest, args[i+1:]...)
			return a, rest
		}
	}
	return "", args
}

// beadRecord is the (subset of) fields we read from .ddx/beads.jsonl. The
// full schema has many more fields; we only need the ones that gate
// promotion.
type beadRecord struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	ClosingCommitSHA string `json:"closing_commit_sha"`
	ParentCommitSHA  string `json:"parent_commit_sha"`
	Title            string `json:"title"`
}

func loadBeadFromJSONL(path, id string) (*beadRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no bead tracker at %s", path)
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// Bead records can be large (multi-paragraph descriptions / acceptance).
	const bigBuf = 4 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), bigBuf)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec beadRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.ID == id {
			return &rec, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return nil, fmt.Errorf("bead %q not found in %s", id, path)
}
