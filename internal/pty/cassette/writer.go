package cassette

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/DocumentDrivenDX/agent/internal/pty/session"
	"github.com/DocumentDrivenDX/agent/internal/pty/terminal"
	"github.com/DocumentDrivenDX/agent/internal/safefs"
)

type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
}

type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }

type RecorderOption func(*recorderConfig)

type recorderConfig struct {
	clock Clock
}

func WithClock(clock Clock) RecorderOption {
	return func(cfg *recorderConfig) {
		cfg.clock = clock
	}
}

type Recorder struct {
	dir         string
	manifest    Manifest
	clock       Clock
	start       time.Time
	seq         uint64
	rawOffset   int64
	rawHash     hash.Hash
	raw         *os.File
	input       *json.Encoder
	output      *json.Encoder
	frames      *json.Encoder
	service     *json.Encoder
	inputFile   *os.File
	outputFile  *os.File
	framesFile  *os.File
	serviceFile *os.File
	closed      bool
}

func Create(dir string, manifest Manifest, opts ...RecorderOption) (*Recorder, error) {
	cfg := recorderConfig{clock: realClock{}}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.clock == nil {
		cfg.clock = realClock{}
	}
	if err := refuseNewerVersion(dir); err != nil {
		return nil, err
	}
	if err := safefs.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	now := cfg.clock.Now()
	manifest = defaultManifest(manifest, now)
	r := &Recorder{
		dir:      dir,
		manifest: manifest,
		clock:    cfg.clock,
		start:    now,
		rawHash:  sha256.New(),
	}
	var err error
	if r.raw, err = safefs.Create(filepath.Join(dir, OutputRawFile)); err != nil {
		return nil, err
	}
	if r.inputFile, r.input, err = createJSONL(filepath.Join(dir, InputFile)); err != nil {
		_ = r.Close()
		return nil, err
	}
	if r.outputFile, r.output, err = createJSONL(filepath.Join(dir, OutputEventsFile)); err != nil {
		_ = r.Close()
		return nil, err
	}
	if r.framesFile, r.frames, err = createJSONL(filepath.Join(dir, FramesFile)); err != nil {
		_ = r.Close()
		return nil, err
	}
	if r.serviceFile, r.service, err = createJSONL(filepath.Join(dir, ServiceEventsFile)); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

func (r *Recorder) Manifest() Manifest { return r.manifest }

func (r *Recorder) RecordInput(kind session.EventKind, b []byte, key session.Key, size *session.Size, signal string) (InputRecord, error) {
	rec := InputRecord{
		Timed:  r.nextTimed(),
		Kind:   kind,
		Bytes:  append([]byte(nil), b...),
		Key:    key,
		Size:   size,
		Signal: signal,
	}
	return rec, r.input.Encode(rec)
}

func (r *Recorder) RecordSessionEvent(ev session.Event) error {
	switch ev.Kind {
	case session.EventInput:
		_, err := r.RecordInput(ev.Kind, ev.Bytes, ev.Key, nil, "")
		return err
	case session.EventResize:
		size := ev.Size
		_, err := r.RecordInput(ev.Kind, nil, "", &size, "")
		return err
	case session.EventSignal:
		_, err := r.RecordInput(ev.Kind, nil, "", nil, ev.Signal)
		return err
	case session.EventOutput:
		_, err := r.RecordOutput(session.OutputChunk{Bytes: ev.Bytes})
		return err
	}
	return nil
}

func (r *Recorder) RecordOutput(chunk session.OutputChunk) (OutputRecord, error) {
	offset := r.rawOffset
	if len(chunk.Bytes) > 0 {
		if _, err := r.raw.Write(chunk.Bytes); err != nil {
			return OutputRecord{}, err
		}
		if _, err := r.rawHash.Write(chunk.Bytes); err != nil {
			return OutputRecord{}, err
		}
		r.rawOffset += int64(len(chunk.Bytes))
	}
	sum := sha256.Sum256(chunk.Bytes)
	rec := OutputRecord{
		Timed:  r.nextTimed(),
		Offset: offset,
		Length: int64(len(chunk.Bytes)),
		SHA256: hex.EncodeToString(sum[:]),
	}
	return rec, r.output.Encode(rec)
}

func (r *Recorder) RecordFrame(frame terminal.Frame) (FrameRecord, error) {
	rec := FrameRecord{
		Timed:        r.nextTimed(),
		Size:         frame.Size,
		Text:         append([]string(nil), frame.Text...),
		Cells:        frame.Cells,
		Cursor:       frame.Cursor,
		Title:        frame.Title,
		RawStart:     frame.RawStart,
		RawEnd:       frame.RawEnd,
		ParserOffset: frame.ParserOffset,
	}
	return rec, r.frames.Encode(rec)
}

func (r *Recorder) RecordServiceEvent(payload any) (ServiceEventRecord, error) {
	raw, err := marshalRaw(payload)
	if err != nil {
		return ServiceEventRecord{}, err
	}
	rec := ServiceEventRecord{Timed: r.nextTimed(), Payload: raw}
	return rec, r.service.Encode(rec)
}

func (r *Recorder) RecordFinal(final FinalRecord) error {
	final.Timed = r.nextTimed()
	if final.DurationMS == 0 {
		final.DurationMS = r.clock.Since(r.start).Milliseconds()
	}
	return writeJSON(filepath.Join(r.dir, FinalFile), final)
}

func (r *Recorder) WriteQuota(quota QuotaRecord) error {
	return writeJSON(filepath.Join(r.dir, QuotaFile), quota)
}

func (r *Recorder) WriteDiscovery(discovery DiscoveryRecord) error {
	return writeJSON(filepath.Join(r.dir, DiscoveryFile), discovery)
}

func (r *Recorder) WriteScrubReport(report ScrubReport) error {
	return writeJSON(filepath.Join(r.dir, ScrubReportFile), report)
}

func (r *Recorder) Close() error {
	if r == nil || r.closed {
		return nil
	}
	r.closed = true
	var err error
	for _, f := range []*os.File{r.inputFile, r.outputFile, r.framesFile, r.serviceFile, r.raw} {
		if f != nil {
			if closeErr := f.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
	}
	r.manifest.ContentDigest.SHA256 = hex.EncodeToString(r.rawHash.Sum(nil))
	if _, statErr := os.Stat(filepath.Join(r.dir, FinalFile)); errors.Is(statErr, os.ErrNotExist) {
		return errors.Join(err, fmt.Errorf("cassette: missing required final.json; call RecordFinal before Close"))
	}
	if _, statErr := os.Stat(filepath.Join(r.dir, ScrubReportFile)); errors.Is(statErr, os.ErrNotExist) {
		if writeErr := r.WriteScrubReport(ScrubReport{Status: "clean", Rules: nil, HitCounts: map[string]int{}}); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if writeErr := writeJSON(filepath.Join(r.dir, ManifestFile), r.manifest); writeErr != nil && err == nil {
		err = writeErr
	}
	return err
}

func (r *Recorder) nextTimed() Timed {
	r.seq++
	ms := r.clock.Since(r.start).Milliseconds()
	resolution := r.manifest.Timing.ResolutionMS
	if resolution <= 0 {
		resolution = 100
	}
	ms = (ms / resolution) * resolution
	return Timed{Seq: r.seq, TMS: ms}
}

func defaultManifest(m Manifest, now time.Time) Manifest {
	m.Version = Version
	if m.ID == "" {
		m.ID = randomID()
	}
	if m.Command.WorkdirPolicy == "" {
		m.Command.WorkdirPolicy = "unspecified"
	}
	if m.Terminal.InitialRows == 0 {
		m.Terminal.InitialRows = 24
	}
	if m.Terminal.InitialCols == 0 {
		m.Terminal.InitialCols = 80
	}
	if m.Terminal.Emulator.Name == "" {
		m.Terminal.Emulator = Emulator{Name: "vt10x", Version: "v0.0.0-20220301184237-5011da428d02"}
	}
	if m.Timing.ResolutionMS == 0 {
		m.Timing.ResolutionMS = 100
	}
	if m.Timing.ClockPolicy == "" {
		m.Timing.ClockPolicy = "monotonic-elapsed"
	}
	if m.Timing.ReplayDefault == "" {
		m.Timing.ReplayDefault = ReplayCollapsed
	}
	if m.Provenance.OS == "" {
		m.Provenance.OS = runtime.GOOS
	}
	if m.Provenance.Arch == "" {
		m.Provenance.Arch = runtime.GOARCH
	}
	if m.Provenance.RecordedAt == "" {
		m.Provenance.RecordedAt = now.UTC().Format(time.RFC3339)
	}
	if m.Provenance.RecorderVersion == "" {
		m.Provenance.RecorderVersion = "internal/pty/cassette/v1"
	}
	return m
}

func createJSONL(path string) (*os.File, *json.Encoder, error) {
	f, err := safefs.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, json.NewEncoder(f), nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return safefs.WriteFileAtomic(path, append(data, '\n'), 0o600)
}

func marshalRaw(value any) (json.RawMessage, error) {
	switch v := value.(type) {
	case json.RawMessage:
		return append(json.RawMessage(nil), v...), nil
	case []byte:
		if !json.Valid(v) {
			return nil, fmt.Errorf("service event payload is not valid JSON")
		}
		return append(json.RawMessage(nil), v...), nil
	default:
		b, err := json.Marshal(v)
		return json.RawMessage(b), err
	}
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("cassette-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func refuseNewerVersion(dir string) error {
	data, err := safefs.ReadFile(filepath.Join(dir, ManifestFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var existing struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &existing); err != nil {
		return err
	}
	if existing.Version > Version {
		return fmt.Errorf("cassette: refuse to overwrite newer schema version %d", existing.Version)
	}
	return nil
}
