//go:build windows

package session

import (
	"context"
	"errors"
)

// Start reports the explicit platform gap for this bead. Windows support
// requires a separate OS-specific adapter and fixture bead.
func Start(ctx context.Context, command string, args []string, workdir string, env []string, size Size, opts ...Option) (*Session, error) {
	return nil, errors.New("internal/pty/session: Windows is not supported by this bead")
}
