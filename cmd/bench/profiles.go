package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DocumentDrivenDX/agent/internal/benchmark/profile"
)

const defaultProfilesDir = "scripts/benchmark/profiles"

func cmdProfiles(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s profiles <subcommand>\n\nSubcommands:\n  list   List profiles under scripts/benchmark/profiles/\n", benchCommandName())
		return 2
	}
	switch args[0] {
	case "list":
		return cmdProfilesList(args[1:])
	case "help", "-h", "--help":
		fmt.Fprintf(os.Stderr, "Usage: %s profiles list [--dir <path>] [--json]\n", benchCommandName())
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s profiles: unknown subcommand %q\n", benchCommandName(), args[0])
		return 2
	}
}

func cmdProfilesList(args []string) int {
	fs := flagSet("profiles list")
	dir := fs.String("dir", "", "Profiles directory (default: scripts/benchmark/profiles relative to --work-dir)")
	workDir := fs.String("work-dir", "", "Repository root (default: cwd)")
	jsonOut := fs.Bool("json", false, "Emit JSON array instead of table")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	profilesDir := *dir
	if profilesDir == "" {
		profilesDir = filepath.Join(resolveWorkDir(*workDir), defaultProfilesDir)
	}

	profiles, err := profile.LoadDir(profilesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s profiles list: %v\n", benchCommandName(), err)
		return 1
	}

	if *jsonOut {
		data, _ := json.MarshalIndent(profiles, "", "  ")
		fmt.Println(string(data))
		return 0
	}

	if len(profiles) == 0 {
		fmt.Printf("No profiles found in %s.\n", profilesDir)
		return 0
	}

	fmt.Printf("%-24s %-14s %-32s %-12s %-12s\n", "ID", "PROVIDER", "MODEL", "IN $/Mtok", "OUT $/Mtok")
	fmt.Printf("%-24s %-14s %-32s %-12s %-12s\n", "--", "--------", "-----", "---------", "----------")
	for _, p := range profiles {
		fmt.Printf("%-24s %-14s %-32s %-12.4f %-12.4f\n",
			p.ID,
			string(p.Provider.Type),
			p.Provider.Model,
			p.Pricing.InputUSDPerMTok,
			p.Pricing.OutputUSDPerMTok,
		)
	}
	return 0
}
