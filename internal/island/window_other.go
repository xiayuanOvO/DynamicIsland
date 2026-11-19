//go:build !windows

package island

import "github.com/egoist/mygo"

func roundWindow(*mygo.Window, int) {}

func pointerOverWindow(_ *mygo.Window, fallback bool) bool { return fallback }
