package serviceimpl

import "time"

// Clock is the API-neutral time seam used by service implementation state.
// It deliberately has no dependency on root package contract types so the
// root facade can import this package without creating a cycle.
type Clock func() time.Time

// SystemClock returns the current UTC wall-clock time.
func SystemClock() time.Time {
	return time.Now().UTC()
}

// RuntimeDeps groups constructor dependencies that are safe to pass from the
// root facade into internal implementation packages.
type RuntimeDeps struct {
	Now Clock
}

// Clock returns the configured clock or the system clock when no test seam is
// supplied.
func (d RuntimeDeps) Clock() Clock {
	if d.Now != nil {
		return d.Now
	}
	return SystemClock
}
