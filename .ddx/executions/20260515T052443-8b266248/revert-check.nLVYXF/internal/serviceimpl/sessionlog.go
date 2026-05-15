package serviceimpl

import (
	"fmt"
	"path/filepath"
)

// SessionLogDir resolves the public service session-log directory from the
// per-service override and the loaded config value.
func SessionLogDir(overrideDir, configDir string) string {
	if overrideDir != "" {
		return overrideDir
	}
	return configDir
}

// SessionLogPath resolves the JSONL path for a public service session.
func SessionLogPath(dir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	if dir == "" {
		return "", fmt.Errorf("session log directory is not configured")
	}
	return filepath.Join(dir, sessionID+".jsonl"), nil
}
