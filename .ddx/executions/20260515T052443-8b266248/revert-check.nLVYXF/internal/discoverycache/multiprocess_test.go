package discoverycache

// Multi-process test suite for ADR-012 §9.
//
// Tests that require a separate OS process (SIGKILL, cross-process locking)
// use the standard TestHelperProcess pattern: the test binary spawns itself
// with -test.run=TestHelperProcess and GO_HELPER_PROCESS=1. The helper
// dispatches on os.Args after "--" and calls os.Exit when done.
//
// Single-process tests in this file use goroutines to exercise concurrent
// behaviour within a single process.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// --------------------------------------------------------------------------
// TestMain + TestHelperProcess — helper process boilerplate
// --------------------------------------------------------------------------

func TestMain(m *testing.M) {
	// When spawned as a helper process, TestHelperProcess will call os.Exit.
	os.Exit(m.Run())
}

// TestHelperProcess is the entry point for child processes. It is not a real
// test: when GO_HELPER_PROCESS != "1" it returns immediately (passes).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_HELPER_PROCESS") != "1" {
		return
	}
	// Extract args after "--"
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "helper: no command")
		os.Exit(2)
	}
	switch args[0] {
	case "claim-race":
		helperClaimRace(args[1:])
	case "claim-and-hold":
		helperClaimAndHold(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "helper: unknown command %q\n", args[0])
		os.Exit(2)
	}
	os.Exit(0)
}

// spawnHelper spawns the test binary itself as a helper process.
func spawnHelper(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmdArgs := append([]string{"-test.run=TestHelperProcess", "--"}, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_HELPER_PROCESS=1")
	return cmd
}

// waitForFile polls until path exists or timeout elapses.
func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

// --------------------------------------------------------------------------
// Helper implementations (run inside child processes)
// --------------------------------------------------------------------------

// helperClaimRace: wait for gate file, try to claim, write result.
// Args: cacheRoot tier name ttlMs deadlineMs readyFile startFile resultFile
func helperClaimRace(args []string) {
	if len(args) < 8 {
		fmt.Fprintln(os.Stderr, "claim-race: need 8 args")
		os.Exit(2)
	}
	cacheRoot, tier, name := args[0], args[1], args[2]
	ttlMs, _ := strconv.Atoi(args[3])
	deadlineMs, _ := strconv.Atoi(args[4])
	readyFile, startFile, resultFile := args[5], args[6], args[7]

	c := &Cache{Root: cacheRoot}
	s := Source{
		Tier:            tier,
		Name:            name,
		TTL:             time.Duration(ttlMs) * time.Millisecond,
		RefreshDeadline: time.Duration(deadlineMs) * time.Millisecond,
	}
	_ = os.MkdirAll(filepath.Join(cacheRoot, tier), 0o750)

	// Signal ready.
	_ = os.WriteFile(readyFile, []byte("ready"), 0o600)

	// Poll for start gate (max 10s).
	gateDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(gateDeadline) {
		if _, err := os.Stat(startFile); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	claim, err := c.claimRefresh(s)
	var result string
	switch {
	case err != nil:
		result = "error:" + err.Error()
	case claim.claimed:
		result = "claimed"
		// Hold the marker long enough for the peer to observe it.
		time.Sleep(5 * time.Second)
		c.releaseMarker(s)
	default:
		result = "inflight"
	}
	_ = os.WriteFile(resultFile, []byte(result), 0o600)
}

// helperClaimAndHold: claim source, signal started, then block until killed.
// Args: cacheRoot tier name ttlMs deadlineMs startedFile
func helperClaimAndHold(args []string) {
	if len(args) < 6 {
		fmt.Fprintln(os.Stderr, "claim-and-hold: need 6 args")
		os.Exit(2)
	}
	cacheRoot, tier, name := args[0], args[1], args[2]
	ttlMs, _ := strconv.Atoi(args[3])
	deadlineMs, _ := strconv.Atoi(args[4])
	startedFile := args[5]

	c := &Cache{Root: cacheRoot}
	s := Source{
		Tier:            tier,
		Name:            name,
		TTL:             time.Duration(ttlMs) * time.Millisecond,
		RefreshDeadline: time.Duration(deadlineMs) * time.Millisecond,
	}
	_ = os.MkdirAll(filepath.Join(cacheRoot, tier), 0o750)

	claim, err := c.claimRefresh(s)
	if err != nil || !claim.claimed {
		fmt.Fprintf(os.Stderr, "claim-and-hold: claim failed: %v claimed=%v\n", err, claim.claimed)
		os.Exit(1)
	}
	_ = os.WriteFile(startedFile, []byte("started"), 0o600)
	// Block until SIGKILL.
	select {}
}

// --------------------------------------------------------------------------
// Test 1 — TestConcurrentClaimTwoProcs
// --------------------------------------------------------------------------

// Two child processes race claim_refresh; exactly one returns ClaimedByMe,
// the other AlreadyInFlight.
func TestConcurrentClaimTwoProcs(t *testing.T) {
	dir := t.TempDir()
	tier, name := "discovery", "concurrent"
	ttlMs, deadlineMs := "60000", "60000"

	readyA := filepath.Join(dir, "ready_a")
	readyB := filepath.Join(dir, "ready_b")
	startFile := filepath.Join(dir, "start")
	resultA := filepath.Join(dir, "result_a")
	resultB := filepath.Join(dir, "result_b")

	cmdA := spawnHelper(t, "claim-race", dir, tier, name, ttlMs, deadlineMs, readyA, startFile, resultA)
	cmdB := spawnHelper(t, "claim-race", dir, tier, name, ttlMs, deadlineMs, readyB, startFile, resultB)

	if err := cmdA.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmdA.Process.Kill(); _ = cmdA.Wait() })
	if err := cmdB.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmdB.Process.Kill(); _ = cmdB.Wait() })

	if err := waitForFile(readyA, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := waitForFile(readyB, 10*time.Second); err != nil {
		t.Fatal(err)
	}

	// Release the race — both helpers see start almost simultaneously.
	if err := os.WriteFile(startFile, []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := waitForFile(resultA, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := waitForFile(resultB, 10*time.Second); err != nil {
		t.Fatal(err)
	}

	rawA, _ := os.ReadFile(resultA)
	rawB, _ := os.ReadFile(resultB)
	a, b := string(rawA), string(rawB)

	claimed, inflight := 0, 0
	for _, r := range []string{a, b} {
		switch r {
		case "claimed":
			claimed++
		case "inflight":
			inflight++
		}
	}
	if claimed != 1 || inflight != 1 {
		t.Errorf("expected 1 claimed + 1 inflight; got A=%q B=%q", a, b)
	}
}

// --------------------------------------------------------------------------
// Test 2 — TestReaderDuringRefresh
// --------------------------------------------------------------------------

// 100 concurrent readers observe only valid JSON while a writer produces
// 100 versions via refreshAndCommit.
func TestReaderDuringRefresh(t *testing.T) {
	c := newTestCache(t)
	s := testSource("reader-during-refresh", time.Hour, 10*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	makePayload := func(v int) []byte {
		return []byte(fmt.Sprintf(`{"version":%d,"pad":"%s"}`, v, strings.Repeat("x", 512)))
	}
	if err := atomicWrite(c.dataPath(s), makePayload(0)); err != nil {
		t.Fatal(err)
	}

	const numReaders = 100
	const numVersions = 100

	stop := make(chan struct{})
	errs := make(chan string, numReaders*10)
	var wg sync.WaitGroup

	for range numReaders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				data, err := os.ReadFile(c.dataPath(s))
				if err != nil {
					continue
				}
				var obj map[string]interface{}
				if jerr := json.Unmarshal(data, &obj); jerr != nil {
					errs <- fmt.Sprintf("torn read: %v", jerr)
				}
			}
		}()
	}

	for v := 1; v <= numVersions; v++ {
		_ = c.Refresh(s, func(_ context.Context) ([]byte, error) {
			return makePayload(v), nil
		})
	}

	close(stop)
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

// --------------------------------------------------------------------------
// Test 3 — TestCrashDuringRefresh
// --------------------------------------------------------------------------

// A child process claims and is killed (SIGKILL). The next process detects
// the dead PID in the marker and successfully claims the source.
func TestCrashDuringRefresh(t *testing.T) {
	dir := t.TempDir()
	tier, name := "discovery", "crash-test"
	ttlMs, deadlineMs := "60000", "60000"
	startedFile := filepath.Join(dir, "started")

	cmd := spawnHelper(t, "claim-and-hold", dir, tier, name, ttlMs, deadlineMs, startedFile)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	if err := waitForFile(startedFile, 10*time.Second); err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}

	helperPID := cmd.Process.Pid
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	// Verify dead.
	if processAlive(helperPID) {
		t.Fatal("expected helper to be dead after SIGKILL")
	}

	// claimRefresh should succeed — dead PID recovery.
	c := &Cache{Root: dir}
	s := Source{Tier: tier, Name: name, TTL: time.Hour, RefreshDeadline: 60 * time.Second}
	claim, err := c.claimRefresh(s)
	if err != nil {
		t.Fatalf("claimRefresh after crash: %v", err)
	}
	if !claim.claimed {
		t.Error("expected ClaimedByMe after dead-PID recovery, got AlreadyInFlight")
	}
	c.releaseMarker(s)
}

// --------------------------------------------------------------------------
// Test 4 — TestRefreshTimeout
// --------------------------------------------------------------------------

// A marker whose deadline has expired (past 2×RefreshDeadline threshold) is
// treated as stale even when the PID is alive (this process). The next caller
// claims the source successfully.
func TestRefreshTimeout(t *testing.T) {
	c := newTestCache(t)
	s := testSource("timeout", time.Hour, 1*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	// Deadline expired more than 2×RefreshDeadline ago.
	expiredDeadline := time.Now().Add(-3 * time.Second)
	m := &refreshMarker{
		PID:       os.Getpid(), // alive PID — simulates a process that timed out
		StartedAt: expiredDeadline.Add(-time.Second),
		Deadline:  expiredDeadline,
	}
	if err := writeMarker(c.markerPath(s), m); err != nil {
		t.Fatal(err)
	}

	claim, err := c.claimRefresh(s)
	if err != nil {
		t.Fatalf("claimRefresh: %v", err)
	}
	if !claim.claimed {
		t.Error("expected ClaimedByMe for expired-deadline marker")
	}
	c.releaseMarker(s)
}

// --------------------------------------------------------------------------
// Test 5 — TestForceRefreshWaitsAndReadsFresh
// --------------------------------------------------------------------------

// Refresh (force) with an in-flight refresh: singleflight deduplicates so the
// second caller waits for the first goroutine to finish and reads fresh data.
func TestForceRefreshWaitsAndReadsFresh(t *testing.T) {
	c := newTestCache(t)
	s := testSource("force-wait", time.Hour, 5*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	const staleData = `{"v":"stale"}`
	const freshData = `{"v":"fresh"}`

	// Write stale data.
	if err := atomicWrite(c.dataPath(s), []byte(staleData)); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(c.dataPath(s), past, past)

	started := make(chan struct{})

	// Goroutine A: slow refresh (~300 ms).
	go func() {
		_ = c.Refresh(s, func(_ context.Context) ([]byte, error) {
			close(started)
			time.Sleep(300 * time.Millisecond)
			return []byte(freshData), nil
		})
	}()
	<-started // A has started; it holds the singleflight key.

	// Goroutine B (this goroutine): force-Refresh should wait for A.
	begin := time.Now()
	if err := c.Refresh(s, func(_ context.Context) ([]byte, error) {
		// B's fn is deduplicated by singleflight; it is never called.
		return []byte(`{"v":"b"}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(begin)

	if elapsed < 200*time.Millisecond {
		t.Errorf("Refresh returned in %v, expected to wait ≥ 200ms for in-flight refresh", elapsed)
	}

	res, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Data) != freshData {
		t.Errorf("after force-Refresh, Read() = %q, want %q", res.Data, freshData)
	}
}

// --------------------------------------------------------------------------
// Test 6 — TestConcurrentNormalAndForce
// --------------------------------------------------------------------------

// Normal Read returns stale data immediately without blocking; force Refresh
// completes and leaves fresh data on disk.
func TestConcurrentNormalAndForce(t *testing.T) {
	c := newTestCache(t)
	s := testSource("normal-force", time.Hour, 5*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	const staleData = `{"v":"stale"}`
	const freshData = `{"v":"fresh"}`

	if err := atomicWrite(c.dataPath(s), []byte(staleData)); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(c.dataPath(s), past, past)

	// Normal Read must return stale data immediately (≤ 10 ms).
	start := time.Now()
	res, err := c.Read(s)
	readLatency := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if readLatency > 10*time.Millisecond {
		t.Errorf("Read() took %v, want ≤ 10ms", readLatency)
	}
	if string(res.Data) != staleData {
		t.Errorf("Read() = %q, want stale data", res.Data)
	}
	if !res.Stale {
		t.Error("expected Stale=true")
	}

	// Force-Refresh writes fresh data.
	if err := c.Refresh(s, func(_ context.Context) ([]byte, error) {
		return []byte(freshData), nil
	}); err != nil {
		t.Fatal(err)
	}

	res2, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(res2.Data) != freshData {
		t.Errorf("after Refresh, Read() = %q, want %q", res2.Data, freshData)
	}
}

// --------------------------------------------------------------------------
// Test 7 — TestAtomicRenameVerified
// --------------------------------------------------------------------------

// 100 concurrent readers + 1 writer × 100 versions; every read returns a
// complete, consistent version — no partial write ever observable.
func TestAtomicRenameVerified(t *testing.T) {
	c := newTestCache(t)
	s := testSource("atomic-rename", time.Hour, 10*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	const numVersions = 100
	const numReaders = 100
	const padSize = 2048

	// Build the known valid payloads; each has a unique "v" and distinct padding.
	payloads := make([][]byte, numVersions)
	for i := range payloads {
		payloads[i] = []byte(fmt.Sprintf(`{"v":%d,"pad":"%s"}`, i,
			strings.Repeat(fmt.Sprintf("%05d", i), padSize/5)))
	}
	if err := atomicWrite(c.dataPath(s), payloads[0]); err != nil {
		t.Fatal(err)
	}

	// Index the payloads for O(1) membership test.
	valid := make(map[string]bool, numVersions)
	for _, p := range payloads {
		valid[string(p)] = true
	}

	stop := make(chan struct{})
	errs := make(chan string, numReaders*10)
	var wg sync.WaitGroup

	for range numReaders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				data, err := os.ReadFile(c.dataPath(s))
				if err != nil {
					continue
				}
				if !valid[string(data)] {
					errs <- fmt.Sprintf("invalid/torn payload len=%d: %.80s", len(data), data)
				}
			}
		}()
	}

	for i := 1; i < numVersions; i++ {
		if err := atomicWrite(c.dataPath(s), payloads[i]); err != nil {
			close(stop)
			t.Fatal(err)
		}
	}

	close(stop)
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

// --------------------------------------------------------------------------
// Test 8 — TestPruneDoesNotRaceActiveSources
// --------------------------------------------------------------------------

// Prune skips a source whose .refreshing marker is active (PID alive, deadline
// in the future). None of the source's files are removed.
func TestPruneDoesNotRaceActiveSources(t *testing.T) {
	c := newTestCache(t)
	s := Source{Tier: "discovery", Name: "active-during-prune", TTL: time.Hour, RefreshDeadline: 60 * time.Second}
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	if err := atomicWrite(c.dataPath(s), []byte(`{"preserved":true}`)); err != nil {
		t.Fatal(err)
	}
	m := &refreshMarker{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
		Deadline:  time.Now().UTC().Add(60 * time.Second),
	}
	if err := writeMarker(c.markerPath(s), m); err != nil {
		t.Fatal(err)
	}

	// Prune with no active sources — s would normally be removed.
	if err := c.Prune(nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(c.dataPath(s)); os.IsNotExist(err) {
		t.Error("Prune removed data file while .refreshing marker was active")
	}
	if _, err := os.Stat(c.markerPath(s)); os.IsNotExist(err) {
		t.Error("Prune removed .refreshing marker while it was active")
	}
}

// --------------------------------------------------------------------------
// Test 9 — TestStaleWhileRevalidate
// --------------------------------------------------------------------------

// Read returns stale data immediately (≤ 5 ms). A background refresh fires
// via MaybeRefresh. After the refresh completes, subsequent Read returns fresh.
func TestStaleWhileRevalidate(t *testing.T) {
	c := newTestCache(t)
	s := testSource("stale-reval", time.Hour, 5*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	staleData := []byte(`{"v":"stale"}`)
	freshData := []byte(`{"v":"fresh"}`)

	if err := atomicWrite(c.dataPath(s), staleData); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(c.dataPath(s), past, past)

	// Read must return stale data immediately (≤ 5 ms).
	start := time.Now()
	res, err := c.Read(s)
	readLatency := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if readLatency > 5*time.Millisecond {
		t.Errorf("Read() took %v, want ≤ 5ms", readLatency)
	}
	if string(res.Data) != string(staleData) {
		t.Errorf("Read() = %q, want stale", res.Data)
	}
	if !res.Stale {
		t.Error("expected Stale=true for backdated file")
	}

	// Trigger background refresh; MaybeRefresh returns immediately.
	c.MaybeRefresh(s, func(_ context.Context) ([]byte, error) {
		time.Sleep(50 * time.Millisecond)
		return freshData, nil
	})

	// Poll until fresh data appears (max 5 s).
	refreshDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(refreshDeadline) {
		r, _ := c.Read(s)
		if string(r.Data) == string(freshData) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	res2, err := c.Read(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(res2.Data) != string(freshData) {
		t.Errorf("after background refresh, Read() = %q, want fresh", res2.Data)
	}
	if !res2.Fresh {
		t.Errorf("expected Fresh=true after refresh, Age=%v TTL=%v", res2.Age, s.TTL)
	}
}

// --------------------------------------------------------------------------
// Test 10 — TestPIDReuseSafety
// --------------------------------------------------------------------------

// A marker whose PID is "reused" by an unrelated live process (simulated by
// using this process's PID) is treated as stale once the deadline + 2×
// RefreshDeadline has passed. The deadline check is the safety net.
func TestPIDReuseSafety(t *testing.T) {
	c := newTestCache(t)
	s := testSource("pid-reuse", time.Hour, 1*time.Second)
	_ = os.MkdirAll(filepath.Join(c.Root, s.Tier), 0o750)

	// PID = this process (alive = simulates PID reuse); deadline expired > 2×1s ago.
	expiredDeadline := time.Now().Add(-3 * time.Second)
	m := &refreshMarker{
		PID:       os.Getpid(),
		StartedAt: expiredDeadline.Add(-time.Second),
		Deadline:  expiredDeadline,
	}
	if err := writeMarker(c.markerPath(s), m); err != nil {
		t.Fatal(err)
	}

	// Verify the PID is indeed alive (confirming PID-reuse scenario).
	if !processAlive(os.Getpid()) {
		t.Fatal("precondition: current PID must be alive")
	}
	// But syscall.Kill check alone would pass (PID alive); verify isStale
	// returns true via the deadline check.
	if !isStale(s, m) {
		t.Fatal("precondition: isStale must return true for expired marker")
	}

	claim, err := c.claimRefresh(s)
	if err != nil {
		t.Fatalf("claimRefresh: %v", err)
	}
	if !claim.claimed {
		t.Error("expected ClaimedByMe: PID alive but deadline expired; deadline check is the safety net")
	}
	c.releaseMarker(s)
}

// --------------------------------------------------------------------------
// Suppress unused import warning for syscall (used by processAlive in lock.go)
// --------------------------------------------------------------------------

var _ = syscall.SIGKILL
