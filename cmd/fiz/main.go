// Command fiz is a standalone CLI that wraps the fizeau library.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/DocumentDrivenDX/fizeau/agentcli"
)

// Version info set via -ldflags.
var (
	Version   = "dev"
	BuildTime = ""
	GitCommit = ""
)

func main() {
	version := effectiveVersion(Version)
	cmd := agentcli.MountCLI(
		agentcli.WithStdin(os.Stdin),
		agentcli.WithStdout(os.Stdout),
		agentcli.WithStderr(os.Stderr),
		agentcli.WithVersion(version, BuildTime, GitCommit),
	)
	args := os.Args[1:]
	if agentcli.NeedsLegacyPassthrough(args) {
		args = append([]string{"--"}, args...)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		if code, ok := agentcli.ExitCode(err); ok {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func effectiveVersion(defaultVersion string) string {
	if defaultVersion != "" && defaultVersion != "dev" {
		return defaultVersion
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return defaultVersion
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return defaultVersion
	}
	return info.Main.Version
}
