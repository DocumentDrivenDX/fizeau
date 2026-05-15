// Package terminal derives rendered VT frames from raw PTY bytes.
//
// See internal/pty/doc.go for the governing ADR/SPIKE citations that authorize
// this vt10x-backed emulator wrapper.
package terminal

import (
	"strings"
	"time"

	"github.com/hinshun/vt10x"
)

// Size is a terminal rows/columns pair.
type Size struct {
	Rows int
	Cols int
}

// Clock lets tests inject deterministic frame timestamps.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Cell is one rendered screen cell.
type Cell struct {
	Char rune
	Mode int16
	FG   uint32
	BG   uint32
}

// Cursor captures cursor metadata.
type Cursor struct {
	Row     int
	Col     int
	Visible bool
}

// Frame is a deterministic rendered VT snapshot.
type Frame struct {
	Seq          uint64
	At           time.Time
	TMS          int64
	Size         Size
	Text         []string
	Cells        [][]Cell
	Cursor       Cursor
	Title        string
	RawStart     int64
	RawEnd       int64
	ParserOffset int64
}

// Normalizer rewrites volatile frame data without mutating raw evidence.
type Normalizer func(Frame) Frame

// Emulator is the internal terminal rendering interface.
type Emulator interface {
	Feed([]byte) (Frame, error)
	Resize(Size) Frame
	Snapshot() Frame
	PendingBytes() []byte
}

// Option configures New.
type Option func(*config)

type config struct {
	clock      Clock
	normalizer []Normalizer
}

// WithClock injects a deterministic clock.
func WithClock(clock Clock) Option {
	return func(cfg *config) { cfg.clock = clock }
}

// WithNormalizer adds a volatile-content normalization hook.
func WithNormalizer(n Normalizer) Option {
	return func(cfg *config) {
		if n != nil {
			cfg.normalizer = append(cfg.normalizer, n)
		}
	}
}

// VT10x wraps the selected real VT/ANSI emulator backend.
type VT10x struct {
	term       vt10x.Terminal
	clock      Clock
	start      time.Time
	seq        uint64
	rawOffset  int64
	pending    []byte
	normalize  []Normalizer
	lastRawBeg int64
	lastRawEnd int64
}

// New creates a vt10x-backed emulator.
func New(size Size, opts ...Option) *VT10x {
	cfg := config{clock: realClock{}}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.clock == nil {
		cfg.clock = realClock{}
	}
	if size.Rows <= 0 {
		size.Rows = 24
	}
	if size.Cols <= 0 {
		size.Cols = 80
	}
	start := cfg.clock.Now()
	return &VT10x{
		term:      vt10x.New(vt10x.WithSize(size.Cols, size.Rows)),
		clock:     cfg.clock,
		start:     start,
		normalize: cfg.normalizer,
	}
}

// Backend returns the wrapped vt10x terminal for low-level diagnostics.
func (v *VT10x) Backend() vt10x.Terminal { return v.term }

// Feed writes raw PTY bytes into the emulator and returns the resulting frame.
func (v *VT10x) Feed(b []byte) (Frame, error) {
	start := v.rawOffset
	v.rawOffset += int64(len(b))
	v.lastRawBeg = start
	v.lastRawEnd = v.rawOffset
	data := append([]byte(nil), v.pending...)
	data = append(data, b...)
	written, err := v.term.Write(data)
	if written < len(data) {
		v.pending = append([]byte(nil), data[written:]...)
	} else {
		v.pending = v.pending[:0]
	}
	v.seq++
	frame := v.snapshot()
	frame.RawStart = start
	frame.RawEnd = v.rawOffset
	frame.ParserOffset = v.rawOffset - int64(len(v.pending))
	return frame, err
}

// Resize updates the emulator size and returns a new frame.
func (v *VT10x) Resize(size Size) Frame {
	if size.Rows <= 0 {
		size.Rows = 24
	}
	if size.Cols <= 0 {
		size.Cols = 80
	}
	v.term.Resize(size.Cols, size.Rows)
	v.seq++
	return v.snapshot()
}

// Snapshot returns the current rendered state.
func (v *VT10x) Snapshot() Frame {
	v.seq++
	return v.snapshot()
}

// PendingBytes returns incomplete escape/UTF-8 bytes retained for the next feed.
func (v *VT10x) PendingBytes() []byte {
	return append([]byte(nil), v.pending...)
}

func (v *VT10x) snapshot() Frame {
	now := v.clock.Now()
	cols, rows := v.term.Size()
	c := v.term.Cursor()
	frame := Frame{
		Seq:          v.seq,
		At:           now,
		TMS:          now.Sub(v.start).Milliseconds(),
		Size:         Size{Rows: rows, Cols: cols},
		Text:         make([]string, rows),
		Cells:        make([][]Cell, rows),
		Cursor:       Cursor{Row: c.Y, Col: c.X, Visible: v.term.CursorVisible()},
		Title:        v.term.Title(),
		RawStart:     v.lastRawBeg,
		RawEnd:       v.lastRawEnd,
		ParserOffset: v.rawOffset - int64(len(v.pending)),
	}
	for y := 0; y < rows; y++ {
		var line strings.Builder
		frame.Cells[y] = make([]Cell, cols)
		for x := 0; x < cols; x++ {
			g := v.term.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			line.WriteRune(ch)
			frame.Cells[y][x] = Cell{Char: ch, Mode: g.Mode, FG: uint32(g.FG), BG: uint32(g.BG)}
		}
		frame.Text[y] = strings.TrimRight(line.String(), " ")
	}
	for _, n := range v.normalize {
		frame = n(frame)
	}
	return frame
}
