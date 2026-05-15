package cassette

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/easel/fizeau/internal/safefs"
)

type OpenOption func(*openConfig)

type openConfig struct {
	expectedID       string
	expectedSHA256   string
	requiredEmulator *Emulator
}

func WithExpectedBinding(id, sha256 string) OpenOption {
	return func(cfg *openConfig) {
		cfg.expectedID = id
		cfg.expectedSHA256 = sha256
	}
}

func WithRequiredEmulator(emulator Emulator) OpenOption {
	return func(cfg *openConfig) {
		cfg.requiredEmulator = &emulator
	}
}

type Reader struct {
	dir       string
	manifest  Manifest
	raw       []byte
	inputs    []InputRecord
	outputs   []OutputRecord
	frames    []FrameRecord
	services  []ServiceEventRecord
	final     FinalRecord
	quota     *QuotaRecord
	discovery *DiscoveryRecord
	report    ScrubReport
}

func Open(dir string, opts ...OpenOption) (*Reader, error) {
	cfg := openConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	r := &Reader{dir: dir}
	if err := readJSON(filepath.Join(dir, ManifestFile), &r.manifest); err != nil {
		return nil, err
	}
	if err := validateManifest(r.manifest, cfg); err != nil {
		return nil, err
	}
	required := []string{InputFile, OutputRawFile, OutputEventsFile, FramesFile, ServiceEventsFile, FinalFile, ScrubReportFile}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return nil, fmt.Errorf("cassette: required artifact %s: %w", name, err)
		}
	}
	var err error
	if r.raw, err = safefs.ReadFile(filepath.Join(dir, OutputRawFile)); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(r.raw)
	gotDigest := hex.EncodeToString(sum[:])
	if r.manifest.ContentDigest.SHA256 != gotDigest {
		return nil, fmt.Errorf("cassette: output digest mismatch: manifest=%s actual=%s", r.manifest.ContentDigest.SHA256, gotDigest)
	}
	if err := readJSONL(filepath.Join(dir, InputFile), &r.inputs); err != nil {
		return nil, err
	}
	if err := readJSONL(filepath.Join(dir, OutputEventsFile), &r.outputs); err != nil {
		return nil, err
	}
	if err := readJSONL(filepath.Join(dir, FramesFile), &r.frames); err != nil {
		return nil, err
	}
	if err := readJSONL(filepath.Join(dir, ServiceEventsFile), &r.services); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, FinalFile), &r.final); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, ScrubReportFile), &r.report); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, QuotaFile), &r.quota); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, DiscoveryFile), &r.discovery); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := r.validateEvents(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reader) Manifest() Manifest           { return r.manifest }
func (r *Reader) RawOutput() []byte            { return append([]byte(nil), r.raw...) }
func (r *Reader) Inputs() []InputRecord        { return append([]InputRecord(nil), r.inputs...) }
func (r *Reader) OutputChunks() []OutputRecord { return append([]OutputRecord(nil), r.outputs...) }
func (r *Reader) Frames() []FrameRecord        { return append([]FrameRecord(nil), r.frames...) }
func (r *Reader) ServiceEvents() []ServiceEventRecord {
	return append([]ServiceEventRecord(nil), r.services...)
}
func (r *Reader) Final() FinalRecord          { return r.final }
func (r *Reader) Quota() *QuotaRecord         { return r.quota }
func (r *Reader) Discovery() *DiscoveryRecord { return r.discovery }
func (r *Reader) ScrubReport() ScrubReport    { return r.report }

func (r *Reader) OutputBytes(rec OutputRecord) ([]byte, error) {
	if rec.Offset < 0 || rec.Length < 0 || rec.Offset+rec.Length > int64(len(r.raw)) {
		return nil, fmt.Errorf("cassette: output chunk out of range seq=%d", rec.Seq)
	}
	return append([]byte(nil), r.raw[rec.Offset:rec.Offset+rec.Length]...), nil
}

func (r *Reader) Events() ([]Event, error) {
	events := make([]Event, 0, len(r.inputs)+len(r.outputs)+len(r.frames)+len(r.services)+1)
	for i := range r.inputs {
		rec := r.inputs[i]
		events = append(events, Event{Seq: rec.Seq, TMS: rec.TMS, Kind: EventInput, Input: &rec})
	}
	for i := range r.outputs {
		rec := r.outputs[i]
		events = append(events, Event{Seq: rec.Seq, TMS: rec.TMS, Kind: EventOutput, Output: &rec})
	}
	for i := range r.frames {
		rec := r.frames[i]
		events = append(events, Event{Seq: rec.Seq, TMS: rec.TMS, Kind: EventFrame, Frame: &rec})
	}
	for i := range r.services {
		rec := r.services[i]
		events = append(events, Event{Seq: rec.Seq, TMS: rec.TMS, Kind: EventServiceEvent, Service: &rec})
	}
	final := r.final
	events = append(events, Event{Seq: final.Seq, TMS: final.TMS, Kind: EventFinal, Final: &final})
	sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	for i, ev := range events {
		want := uint64(i + 1)
		if ev.Seq != want {
			return nil, fmt.Errorf("cassette: non-contiguous event seq got=%d want=%d", ev.Seq, want)
		}
	}
	return events, nil
}

func (r *Reader) Replay(ctx context.Context, opts ReplayOptions, sink func(Event) error) error {
	events, err := r.Events()
	if err != nil {
		return err
	}
	mode := opts.Mode
	if mode == "" {
		mode = r.manifest.Timing.ReplayDefault
	}
	if mode == "" {
		mode = ReplayCollapsed
	}
	scale := opts.Scale
	if scale == 0 {
		scale = 1
	}
	sleeper := opts.Sleeper
	if sleeper == nil {
		sleeper = realSleeper{}
	}
	var prev int64
	for i, ev := range events {
		if i > 0 && mode != ReplayCollapsed {
			delay := time.Duration(ev.TMS-prev) * time.Millisecond
			if mode == ReplayScaled {
				delay = time.Duration(float64(delay) * scale)
			}
			if delay > 0 {
				if err := sleeper.Sleep(ctx, delay); err != nil {
					return err
				}
			}
		}
		if err := sink(ev); err != nil {
			return err
		}
		prev = ev.TMS
	}
	return nil
}

type ReplayOptions struct {
	Mode    ReplayMode
	Scale   float64
	Sleeper Sleeper
}

type realSleeper struct{}

func (realSleeper) Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateManifest(m Manifest, cfg openConfig) error {
	if m.Version <= 0 {
		return fmt.Errorf("cassette: missing manifest.version")
	}
	if m.Version > Version {
		return fmt.Errorf("cassette: unsupported manifest.version %d", m.Version)
	}
	if m.ID == "" {
		return fmt.Errorf("cassette: missing manifest.id")
	}
	if m.ContentDigest.SHA256 == "" {
		return fmt.Errorf("cassette: missing manifest.content_digest.sha256")
	}
	if len(m.RequiredFeatures) > 0 {
		return fmt.Errorf("cassette: unknown required feature flags: %v", m.RequiredFeatures)
	}
	if m.Terminal.Emulator.Name == "" || m.Terminal.Emulator.Version == "" {
		return fmt.Errorf("cassette: missing manifest.terminal.emulator")
	}
	if cfg.expectedID != "" && cfg.expectedID != m.ID {
		return fmt.Errorf("cassette: manifest id mismatch: got %s want %s", m.ID, cfg.expectedID)
	}
	if cfg.expectedSHA256 != "" && cfg.expectedSHA256 != m.ContentDigest.SHA256 {
		return fmt.Errorf("cassette: manifest digest mismatch: got %s want %s", m.ContentDigest.SHA256, cfg.expectedSHA256)
	}
	if cfg.requiredEmulator != nil {
		if *cfg.requiredEmulator != m.Terminal.Emulator {
			return fmt.Errorf("cassette: emulator mismatch: got %+v want %+v", m.Terminal.Emulator, *cfg.requiredEmulator)
		}
	}
	if m.Timing.ResolutionMS <= 0 {
		return fmt.Errorf("cassette: invalid manifest.timing.resolution_ms")
	}
	return nil
}

func (r *Reader) validateEvents() error {
	var lastSeq uint64
	var lastTMS int64
	events, err := r.Events()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.Seq <= lastSeq {
			return fmt.Errorf("cassette: event seq regressed")
		}
		if ev.TMS < lastTMS {
			return fmt.Errorf("cassette: event t_ms regressed")
		}
		lastSeq = ev.Seq
		lastTMS = ev.TMS
	}
	for _, out := range r.outputs {
		b, err := r.OutputBytes(out)
		if err != nil {
			return err
		}
		if out.SHA256 != "" {
			sum := sha256.Sum256(b)
			if out.SHA256 != hex.EncodeToString(sum[:]) {
				return fmt.Errorf("cassette: chunk digest mismatch seq=%d", out.Seq)
			}
		}
	}
	return nil
}

func readJSON(path string, target any) error {
	data, err := safefs.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(target)
}

func readJSONL[T any](path string, target *[]T) error {
	f, err := os.Open(path) // #nosec G304 -- cassette readers intentionally inspect user-selected artifact dirs.
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var value T
		if err := dec.Decode(&value); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("%s: %w", path, err)
		}
		*target = append(*target, value)
	}
}
