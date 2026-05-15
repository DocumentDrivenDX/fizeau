// Package picompat provides compatibility for importing configurations from pi.
package picompat

import (
	"os"
	"path/filepath"
)

// DefaultPiDir returns the default pi config directory.
func DefaultPiDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi")
}

// CheckExists checks if pi config directory and auth.json exist.
func CheckExists() bool {
	piDir := DefaultPiDir()
	if piDir == "" {
		return false
	}
	_, err := os.Stat(piDir + "/agent/auth.json")
	return err == nil
}
