//go:build !darwin && !windows

package island

func primaryMouseDown() bool { return false }
