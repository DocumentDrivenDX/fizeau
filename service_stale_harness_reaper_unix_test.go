//go:build !windows

package fizeau

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestServiceStartupReapsStaleHarnessSessions(t *testing.T) {
	cmd := startReaperTestProcess(t)
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	dir := t.TempDir()
	logDir := filepath.Join(dir, "sessions")
	recordPath := filepath.Join(dir, "harness-sessions", "stale.json")
	if err := writeStaleHarnessRecord(recordPath, staleHarnessRecord{
		SessionID: "stale-session",
		Harness:   "codex",
		Command:   "sleep",
		PID:       cmd.Process.Pid,
		PGID:      pgid,
		StartedAt: time.Now().Add(-time.Hour).UTC(),
	}); err != nil {
		t.Fatalf("write record: %v", err)
	}

	_, err = New(ServiceOptions{
		SessionLogDir:           logDir,
		StaleHarnessReaperGrace: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	waitForProcessExit(t, cmd)
	if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
		t.Fatalf("record was not removed, stat err=%v", err)
	}
}

func TestStaleHarnessReaperIgnoresUnownedProcesses(t *testing.T) {
	cmd := startReaperTestProcess(t)
	defer terminateOwnedProcessGroup(mustPGID(t, cmd))
	dir := t.TempDir()

	if err := reapStaleHarnessRecords(filepath.Join(dir, "harness-sessions"), time.Millisecond, time.Now().UTC()); err != nil {
		t.Fatalf("reap records: %v", err)
	}
	if !processGroupAlive(mustPGID(t, cmd)) {
		t.Fatal("unowned process group was terminated")
	}
}

func TestStaleHarnessReaperRemovesDeadPidRecords(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "dead.json")
	if err := writeStaleHarnessRecord(recordPath, staleHarnessRecord{
		SessionID: "dead-session",
		Harness:   "codex",
		Command:   "sleep",
		PID:       999999,
		PGID:      999999,
		StartedAt: time.Now().Add(-time.Hour).UTC(),
	}); err != nil {
		t.Fatalf("write record: %v", err)
	}
	if err := reapStaleHarnessRecords(dir, time.Millisecond, time.Now().UTC()); err != nil {
		t.Fatalf("reap records: %v", err)
	}
	if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
		t.Fatalf("dead record was not removed, stat err=%v", err)
	}
}

func startReaperTestProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 300")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
				terminateOwnedProcessGroup(pgid)
			}
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd
}

func mustPGID(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Getpgid: %v", err)
	}
	return pgid
}

func waitForProcessExit(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stale process did not exit")
	}
}
