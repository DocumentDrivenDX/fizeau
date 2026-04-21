package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/hinshun/vt10x"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time {
	f.t = f.t.Add(100 * time.Millisecond)
	return f.t
}

func TestVT10xFrameDerivationCursorClearStyleAndUnicode(t *testing.T) {
	clock := &fakeClock{}
	emu := New(Size{Rows: 4, Cols: 20}, WithClock(clock))

	frame, err := emu.Feed([]byte("hello\nwide: 界"))
	require.NoError(t, err)
	require.Equal(t, uint64(1), frame.Seq)
	require.Equal(t, int64(100), frame.TMS)
	require.Contains(t, frame.Text[0], "hello")
	require.Contains(t, frame.Text[1], "wide: 界")
	require.Equal(t, 1, frame.Cursor.Row)
	require.Greater(t, frame.Cursor.Col, 0)
	require.Equal(t, int64(0), frame.RawStart)
	require.Equal(t, int64(len("hello\nwide: 界")), frame.RawEnd)

	frame, err = emu.Feed([]byte("\x1b[31mred\x1b[0m"))
	require.NoError(t, err)
	require.Contains(t, strings.Join(frame.Text, "\n"), "red")
	var styled bool
	for _, row := range frame.Cells {
		for _, cell := range row {
			if cell.Char == 'r' && cell.FG != uint32(vt10x.DefaultFG) {
				styled = true
			}
		}
	}
	require.True(t, styled, "style metadata should be preserved on rendered cells")

	_, err = emu.Feed([]byte("\x1b["))
	require.NoError(t, err)
	frame, err = emu.Feed([]byte("2J\x1b[Hafter-clear"))
	require.NoError(t, err)
	require.Contains(t, frame.Text[0], "after-clear")
	require.NotContains(t, strings.Join(frame.Text, "\n"), "hello")
	require.Equal(t, frame.RawEnd, frame.ParserOffset)
}

func TestVT10xResizeSnapshotAndNormalization(t *testing.T) {
	clock := &fakeClock{}
	emu := New(
		Size{Rows: 2, Cols: 10},
		WithClock(clock),
		WithNormalizer(func(f Frame) Frame {
			for i := range f.Text {
				f.Text[i] = strings.ReplaceAll(f.Text[i], "PID 123", "PID <pid>")
			}
			return f
		}),
	)
	_, err := emu.Feed([]byte("PID 123"))
	require.NoError(t, err)

	frame := emu.Resize(Size{Rows: 3, Cols: 12})
	require.Equal(t, Size{Rows: 3, Cols: 12}, frame.Size)
	require.Contains(t, frame.Text[0], "PID <pid>")

	snap := emu.Snapshot()
	require.Equal(t, Size{Rows: 3, Cols: 12}, snap.Size)
	require.Greater(t, snap.Seq, frame.Seq)
}
