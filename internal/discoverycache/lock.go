package discoverycache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

type lockPayload struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// acquireLock acquires the tier-1 brief mutation lock for s. Returns a release
// func on success. Polls up to lockAcquisitionTimeout, recovering stale locks
// whose owner PID is dead (mirrors acquireMatrixLock semantics from
// cmd/bench/matrix.go:1222).
func (c *Cache) acquireLock(s Source) (func(), error) {
	lockPath := c.lockPath(s)
	payload := lockPayload{PID: os.Getpid(), StartedAt: time.Now().UTC()}
	raw, _ := json.Marshal(payload)

	deadline := time.Now().Add(lockAcquisitionTimeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304
		if err == nil {
			if _, werr := f.Write(raw); werr != nil {
				_ = f.Close()
				_ = os.Remove(lockPath)
				return nil, werr
			}
			if cerr := f.Close(); cerr != nil {
				_ = os.Remove(lockPath)
				return nil, cerr
			}
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		// Recover stale lock if owner is dead.
		if existing, rerr := readLockPayload(lockPath); rerr == nil && !processAlive(existing.PID) {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("discoverycache: timed out acquiring lock for %s/%s", s.Tier, s.Name)
		}
		time.Sleep(lockPollInterval)
	}
}

func readLockPayload(path string) (lockPayload, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return lockPayload{}, err
	}
	var p lockPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return lockPayload{}, err
	}
	return p, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
