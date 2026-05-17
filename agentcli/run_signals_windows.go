//go:build windows

package agentcli

import "os"

// runCancelSignals returns the OS signals that cancel a fiz run.
// Windows: only Interrupt; SIGTERM has no Windows analogue.
func runCancelSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
