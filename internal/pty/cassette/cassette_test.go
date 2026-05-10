package cassette

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/easel/fizeau/internal/pty/session"
	"github.com/easel/fizeau/internal/pty/terminal"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	now     time.Time
	elapsed time.Duration
	step    time.Duration
	jump    time.Duration
}

func (f *fakeClock) Now() time.Time {
	f.now = f.now.Add(f.jump)
	return f.now
}

func (f *fakeClock) Since(time.Time) time.Duration {
	f.elapsed += f.step
	return f.elapsed
}

type fakeSleeper struct {
	delays []time.Duration
}

func (f *fakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	f.delays = append(f.delays, d)
	return ctx.Err()
}

func TestRecordReadReplaySyntheticTerminalRun(t *testing.T) {
	dir := t.TempDir()
	clock := &fakeClock{now: time.Unix(100, 0), step: 35 * time.Millisecond, jump: time.Hour}
	rec, err := Create(dir, Manifest{
		ID:         "cassette-test",
		Harness:    Harness{Name: "synthetic", BinaryVersion: "v1"},
		Command:    Command{Argv: []string{"sh", "-c", "printf hello"}, WorkdirPolicy: "tempdir", EnvAllowlist: []string{"PATH"}, TimeoutMS: 1000, PermissionMode: "read-only"},
		Terminal:   Terminal{InitialRows: 4, InitialCols: 20, Term: "xterm-256color", Emulator: Emulator{Name: "vt10x", Version: "test"}},
		Provenance: Provenance{GitSHA: "abc123", ContractVersion: "CONTRACT-003"},
	}, WithClock(clock))
	require.NoError(t, err)

	_, err = rec.RecordInput(session.EventInput, []byte("prompt"), "", nil, "")
	require.NoError(t, err)
	_, err = rec.RecordInput(session.EventResize, nil, "", &session.Size{Rows: 5, Cols: 30}, "")
	require.NoError(t, err)
	out, err := rec.RecordOutput(session.OutputChunk{Bytes: []byte("hello\n")})
	require.NoError(t, err)
	emu := terminal.New(terminal.Size{Rows: 4, Cols: 20}, terminal.WithClock(terminalClock{}))
	frame, err := emu.Feed([]byte("hello\n"))
	require.NoError(t, err)
	_, err = rec.RecordFrame(frame)
	require.NoError(t, err)
	_, err = rec.RecordServiceEvent(map[string]any{"type": "routing", "provider": "cassette"})
	require.NoError(t, err)
	require.NoError(t, rec.WriteQuota(QuotaRecord{Source: "record-mode", Status: "captured"}))
	require.NoError(t, rec.WriteScrubReport(ScrubReport{Status: "clean", Rules: []string{"test"}, HitCounts: map[string]int{}}))
	require.NoError(t, rec.RecordFinal(FinalRecord{Exit: &session.ExitStatus{Code: 0, Exited: true}, FinalText: "done"}))
	require.NoError(t, rec.Close())

	require.FileExists(t, filepath.Join(dir, ManifestFile))
	require.FileExists(t, filepath.Join(dir, InputFile))
	require.FileExists(t, filepath.Join(dir, OutputRawFile))
	require.FileExists(t, filepath.Join(dir, OutputEventsFile))
	require.FileExists(t, filepath.Join(dir, FramesFile))
	require.FileExists(t, filepath.Join(dir, ServiceEventsFile))
	require.FileExists(t, filepath.Join(dir, FinalFile))
	require.FileExists(t, filepath.Join(dir, QuotaFile))
	require.FileExists(t, filepath.Join(dir, ScrubReportFile))

	reader, err := Open(dir, WithExpectedBinding("cassette-test", rec.Manifest().ContentDigest.SHA256), WithRequiredEmulator(Emulator{Name: "vt10x", Version: "test"}))
	require.NoError(t, err)
	require.Equal(t, "cassette-test", reader.Manifest().ID)
	require.Equal(t, int64(100), reader.Manifest().Timing.ResolutionMS)
	require.Equal(t, "captured", reader.Quota().Status)
	require.Equal(t, "done", reader.Final().FinalText)

	raw := reader.RawOutput()
	require.Equal(t, []byte("hello\n"), raw)
	chunk, err := reader.OutputBytes(out)
	require.NoError(t, err)
	require.Equal(t, raw, chunk)

	outputJSONL, err := os.ReadFile(filepath.Join(dir, OutputEventsFile))
	require.NoError(t, err)
	require.NotContains(t, string(outputJSONL), "hello")

	events, err := reader.Events()
	require.NoError(t, err)
	require.Len(t, events, 6)
	require.Equal(t, uint64(1), events[0].Seq)
	require.Equal(t, EventFinal, events[len(events)-1].Kind)
	require.Equal(t, int64(0), events[0].TMS)
	require.Equal(t, int64(0), events[1].TMS)

	var collapsed []EventKind
	require.NoError(t, reader.Replay(context.Background(), ReplayOptions{Mode: ReplayCollapsed}, func(ev Event) error {
		collapsed = append(collapsed, ev.Kind)
		return nil
	}))
	require.Equal(t, []EventKind{EventInput, EventInput, EventOutput, EventFrame, EventServiceEvent, EventFinal}, collapsed)

	sleeper := &fakeSleeper{}
	require.NoError(t, reader.Replay(context.Background(), ReplayOptions{Mode: ReplayRealtime, Sleeper: sleeper}, func(Event) error { return nil }))
	require.NotEmpty(t, sleeper.delays)
}

func TestReaderRejectsInvalidSchemaBindingsAndArtifacts(t *testing.T) {
	dir := writeMinimalCassette(t)

	_, err := Open(dir, WithExpectedBinding("wrong", ""))
	require.ErrorContains(t, err, "manifest id mismatch")
	_, err = Open(dir, WithRequiredEmulator(Emulator{Name: "other", Version: "test"}))
	require.ErrorContains(t, err, "emulator mismatch")

	require.NoError(t, os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"version":2}`), 0o600))
	_, err = Open(dir)
	require.ErrorContains(t, err, "unsupported")
	_, err = Create(dir, Manifest{})
	require.ErrorContains(t, err, "refuse to overwrite newer")
}

func TestReaderRejectsMissingRequiredFieldsUnknownFeaturesAndDigestMismatch(t *testing.T) {
	dir := writeMinimalCassette(t)
	var manifest Manifest
	require.NoError(t, readJSON(filepath.Join(dir, ManifestFile), &manifest))
	manifest.ID = ""
	require.NoError(t, writeJSON(filepath.Join(dir, ManifestFile), manifest))
	_, err := Open(dir)
	require.ErrorContains(t, err, "missing manifest.id")

	manifest.ID = "minimal"
	manifest.RequiredFeatures = []string{"future-required"}
	require.NoError(t, writeJSON(filepath.Join(dir, ManifestFile), manifest))
	_, err = Open(dir)
	require.ErrorContains(t, err, "unknown required feature")

	manifest.RequiredFeatures = nil
	manifest.ContentDigest.SHA256 = strings.Repeat("0", 64)
	require.NoError(t, writeJSON(filepath.Join(dir, ManifestFile), manifest))
	_, err = Open(dir)
	require.ErrorContains(t, err, "output digest mismatch")

	manifest.ContentDigest.SHA256 = sha256Hex([]byte("x"))
	require.NoError(t, writeJSON(filepath.Join(dir, ManifestFile), manifest))
	require.NoError(t, os.Remove(filepath.Join(dir, FramesFile)))
	_, err = Open(dir)
	require.ErrorContains(t, err, FramesFile)
}

func TestReaderIgnoresUnknownOptionalFieldsWithinVersion(t *testing.T) {
	dir := writeMinimalCassette(t)
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), `"version": 1,`, `"version": 1, "future_optional": {"ok": true},`, 1))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ManifestFile), data, 0o600))
	_, err = Open(dir)
	require.NoError(t, err)
}

func TestScrubRulesNormalizeSecretsAndVolatileValues(t *testing.T) {
	s := NewScrubber(Scrubber{
		Home:                   "/home/alice",
		Worktree:               "/src/project",
		EnvAllowlist:           []string{"PATH"},
		AccountIdentifiers:     []string{"alice@example.com"},
		IntentionallyPreserved: []string{"exit_code"},
	})
	input := "Bearer SECRET token=abcdefghi alice@example.com /home/alice /src/project 2026-04-21T02:00:00Z 12:01:02 pid=123 42ms /tmp/test.sock /tmp/my-temp-file frame=9 6f9619ff-8b86-d011-b42d-00cf4fc964ff"
	out, report := s.ScrubString(input)
	require.Contains(t, out, "Bearer <redacted>")
	require.Contains(t, out, "token=<redacted>")
	require.Contains(t, out, "<account>")
	require.Contains(t, out, "$HOME")
	require.Contains(t, out, "$WORKTREE")
	require.Contains(t, out, "<timestamp>")
	require.Contains(t, out, "<time>")
	require.Contains(t, out, "pid=<pid>")
	require.Contains(t, out, "<duration>")
	require.Contains(t, out, "<socket>")
	require.Contains(t, out, "<tmpfile>")
	require.Contains(t, out, "frame=<n>")
	require.Contains(t, out, "<uuid>")
	require.Equal(t, "redacted", report.Status)
	require.NotZero(t, report.HitCounts["bearer-token"])

	env, envReport := s.ScrubEnv(map[string]string{"PATH": "/bin", "SECRET": "x"})
	require.Equal(t, map[string]string{"PATH": "/bin"}, env)
	require.Equal(t, "redacted", envReport.Status)
	require.NotEmpty(t, envReport.Rules)
	require.Contains(t, s.NormalizeVolatile("/home/alice alice@example.com"), "$HOME <account>")
}

func TestRecordLockFailsFastAndReplayIsReadOnlyParallelSafe(t *testing.T) {
	root := t.TempDir()
	lock, err := AcquireRecordLock(root, "acct@example.com")
	require.NoError(t, err)
	_, err = AcquireRecordLock(root, "acct@example.com")
	require.ErrorContains(t, err, "already held")
	require.NoError(t, lock.Release())
	lock, err = AcquireRecordLock(root, "acct@example.com")
	require.NoError(t, err)
	require.NoError(t, lock.Release())
	lockA, err := AcquireRecordLock(root, "a/b")
	require.NoError(t, err)
	lockB, err := AcquireRecordLock(root, "a_b")
	require.NoError(t, err)
	require.NoError(t, lockA.Release())
	require.NoError(t, lockB.Release())

	dir := writeMinimalCassette(t)
	reader, err := Open(dir)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, reader.Replay(context.Background(), ReplayOptions{Mode: ReplayCollapsed}, func(Event) error { return nil }))
		}()
	}
	wg.Wait()
}

func TestRecorderRequiresExplicitFinal(t *testing.T) {
	rec, err := Create(t.TempDir(), Manifest{
		ID:       "missing-final",
		Harness:  Harness{Name: "synthetic"},
		Command:  Command{WorkdirPolicy: "tempdir"},
		Terminal: Terminal{InitialRows: 2, InitialCols: 4, Emulator: Emulator{Name: "vt10x", Version: "test"}},
	})
	require.NoError(t, err)
	_, err = rec.RecordOutput(session.OutputChunk{Bytes: []byte("x")})
	require.NoError(t, err)
	require.ErrorContains(t, rec.Close(), "missing required final.json")
}

func TestRequiredEmulatorReportsMissingBeforeMismatch(t *testing.T) {
	dir := writeMinimalCassette(t)
	var manifest Manifest
	require.NoError(t, readJSON(filepath.Join(dir, ManifestFile), &manifest))
	manifest.Terminal.Emulator = Emulator{}
	require.NoError(t, writeJSON(filepath.Join(dir, ManifestFile), manifest))
	_, err := Open(dir, WithRequiredEmulator(Emulator{Name: "vt10x", Version: "test"}))
	require.ErrorContains(t, err, "missing manifest.terminal.emulator")
}

type terminalClock struct{}

func (terminalClock) Now() time.Time { return time.Unix(100, 0) }

func writeMinimalCassette(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rec, err := Create(dir, Manifest{
		ID:       "minimal",
		Harness:  Harness{Name: "synthetic"},
		Command:  Command{WorkdirPolicy: "tempdir"},
		Terminal: Terminal{InitialRows: 2, InitialCols: 4, Emulator: Emulator{Name: "vt10x", Version: "test"}},
	}, WithClock(&fakeClock{now: time.Unix(100, 0), step: 100 * time.Millisecond}))
	require.NoError(t, err)
	_, err = rec.RecordOutput(session.OutputChunk{Bytes: []byte("x")})
	require.NoError(t, err)
	_, err = rec.RecordFrame(terminal.Frame{Size: terminal.Size{Rows: 2, Cols: 4}, Text: []string{"x"}, Cursor: terminal.Cursor{}, RawEnd: 1, ParserOffset: 1})
	require.NoError(t, err)
	_, err = rec.RecordServiceEvent(json.RawMessage(`{"type":"final"}`))
	require.NoError(t, err)
	require.NoError(t, rec.RecordFinal(FinalRecord{Exit: &session.ExitStatus{Code: 0, Exited: true}}))
	require.NoError(t, rec.WriteScrubReport(ScrubReport{Status: "clean", HitCounts: map[string]int{}}))
	require.NoError(t, rec.Close())
	return dir
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
