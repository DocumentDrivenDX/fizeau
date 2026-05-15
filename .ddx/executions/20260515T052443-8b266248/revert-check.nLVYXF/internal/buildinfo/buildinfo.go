// Package buildinfo derives version metadata from ldflags and runtime VCS info.
package buildinfo

import (
	"os/exec"
	"runtime/debug"
	"strings"
)

// Info holds version metadata for the running binary.
type Info struct {
	Version string
	Commit  string
	Built   string
	Dirty   bool
}

// Read returns best-effort build metadata. ldflag values take precedence;
// runtime VCS info from debug.ReadBuildInfo() fills any empty fields.
// For dev builds in git worktrees where the Go toolchain cannot embed VCS
// settings, it falls back to running git directly.
func Read(ldflagVersion, ldflagCommit, ldflagBuilt string) Info {
	info := Info{
		Version: ldflagVersion,
		Commit:  ldflagCommit,
		Built:   ldflagBuilt,
	}
	vcsModifiedKnown := false
	bi, ok := debug.ReadBuildInfo()
	if ok {
		if (info.Version == "" || info.Version == "dev") &&
			bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" && s.Value != "" {
					if len(s.Value) >= 8 {
						info.Commit = s.Value[:8]
					} else {
						info.Commit = s.Value
					}
				}
			case "vcs.time":
				if info.Built == "" {
					info.Built = s.Value
				}
			case "vcs.modified":
				info.Dirty = s.Value == "true"
				vcsModifiedKnown = true
			}
		}
	}
	// Fall back to running git directly when the Go toolchain couldn't embed
	// VCS metadata (e.g., git worktrees, restricted environments).
	if info.Commit == "" {
		if c := gitShortHead(); c != "" {
			info.Commit = c
			if info.Built == "" {
				info.Built = gitHeadTime()
			}
		}
	}
	if !vcsModifiedKnown {
		info.Dirty = gitIsDirty()
	}
	return info
}

// gitShortHead returns the 8-char abbreviated HEAD commit, or empty on failure.
func gitShortHead() string {
	out, err := exec.Command("git", "rev-parse", "--short=8", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitIsDirty reports whether the working tree has uncommitted changes.
func gitIsDirty() bool {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// gitHeadTime returns the committer date of HEAD in RFC3339, or empty on failure.
func gitHeadTime() string {
	out, err := exec.Command("git", "log", "-1", "--format=%cI").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
