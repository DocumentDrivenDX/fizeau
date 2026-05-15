package discoverycache

import (
	"os"
	"path/filepath"
)

// atomicWrite writes data to path using the tmp→fsync→rename pattern so
// readers never observe a partial write. Mirrors the pattern in
// internal/harnesses/claude/quota_cache.go:writeClaudeQuota.
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304
	if err != nil {
		return err
	}
	if _, werr := f.Write(data); werr != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return werr
	}
	if serr := f.Sync(); serr != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return serr
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(tmp)
		return cerr
	}
	if rerr := os.Rename(tmp, path); rerr != nil {
		_ = os.Remove(tmp)
		return rerr
	}
	_ = os.Chmod(path, 0o600) // best-effort: ensure mode even if older file existed
	return nil
}
