//go:build windows

package fizeau

func processGroupAlive(int) bool { return false }

func terminateOwnedProcessGroup(int) {}
