// Command ddx-agent is a standalone CLI that wraps the agent library.
package main

import (
	"fmt"
	"os"

	"github.com/DocumentDrivenDX/agent/agentcli"
)

// Version info set via -ldflags.
var (
	Version   = "dev"
	BuildTime = ""
	GitCommit = ""
)

func main() {
	cmd := agentcli.MountCLI(
		agentcli.WithStdin(os.Stdin),
		agentcli.WithStdout(os.Stdout),
		agentcli.WithStderr(os.Stderr),
		agentcli.WithVersion(Version, BuildTime, GitCommit),
	)
	cmd.SetArgs(append([]string{"--"}, os.Args[1:]...))
	if err := cmd.Execute(); err != nil {
		if code, ok := agentcli.ExitCode(err); ok {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
