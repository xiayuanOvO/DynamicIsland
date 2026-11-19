//go:build !windows && !darwin

package island

func foregroundFullscreen() bool { return false }
