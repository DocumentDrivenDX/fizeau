// Command fiz is a standalone CLI that wraps the fizeau library.
package main

import (
	"fmt"
	"os"

	"github.com/easel/fizeau/agentcli"
	"github.com/easel/fizeau/internal/buildinfo"
)

// Version info set via -ldflags.
var (
	Version   = "dev"
	BuildTime = ""
	GitCommit = ""
)

func main() {
	bi := buildinfo.Read(Version, GitCommit, BuildTime)
	cmd := agentcli.MountCLI(
		agentcli.WithStdin(os.Stdin),
		agentcli.WithStdout(os.Stdout),
		agentcli.WithStderr(os.Stderr),
		agentcli.WithVersion(bi.Version, bi.Built, bi.Commit),
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

// effectiveVersion returns version from ldflags when set, otherwise falls back
// to the module version from runtime build info.
func effectiveVersion(defaultVersion string) string {
	return buildinfo.Read(defaultVersion, "", "").Version
}
