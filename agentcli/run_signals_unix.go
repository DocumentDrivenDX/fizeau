//go:build !windows

package agentcli

import (
	"os"
	"syscall"
)

// runCancelSignals returns the OS signals that cancel a fiz run.
// Unix: both SIGINT (Ctrl-C) and SIGTERM (Harbor/runner cancellation)
// route through the same context-cancellation path so wrapped harness
// runners can kill their subprocess trees before the process exits.
func runCancelSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
