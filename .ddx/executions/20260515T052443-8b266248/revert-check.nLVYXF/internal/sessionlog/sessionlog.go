// Package sessionlog centralizes opening per-request session log files for
// harness runners and the agent service. It exists to localize the file
// permission and path-inclusion safety contract in one place rather than
// scattering nosec annotations across every caller.
package sessionlog

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenAppend creates dir (mode 0o700) if needed and opens or appends to the
// per-session log file "agent-<sessionID>.jsonl" beneath it (mode 0o600).
//
// dir is caller-trusted (set by the agent service via SessionLogDir); sessionID
// is a service-generated identifier (UUID or "<harness>-<unixnano>"). Neither
// is sourced from network input.
func OpenAppend(dir, sessionID string) (*os.File, error) {
	if dir == "" {
		return nil, fmt.Errorf("sessionlog: empty dir")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("sessionlog: empty sessionID")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "agent-"+sessionID+".jsonl")
	// #nosec G304 -- path is constructed from caller-trusted SessionLogDir + a
	// service-generated sessionID; no external input reaches this open.
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}

// EnsureDir creates the session log dir (mode 0o700). It exists for callers
// that need the directory present without opening a file (e.g., to compute a
// path for a stub write).
func EnsureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("sessionlog: empty dir")
	}
	return os.MkdirAll(dir, 0o700)
}
