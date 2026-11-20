//go:build !windows

package island

import (
	"runtime"

	"github.com/egoist/mygo"
)

func configureWindowGlass(*mygo.Window, int) bool { return false }
func configureFloatingWindow(*mygo.Window)        {}

// On macOS the window can resize beneath a stationary pointer.
func pointerOverWindow(win *mygo.Window, fallback bool) bool {
	if runtime.GOOS != "darwin" {
		return fallback
	}
	point, bounds := mygo.Screen.CursorScreenPoint(), win.Bounds()
	return point.X >= bounds.X && point.X < bounds.X+bounds.Width &&
		point.Y >= bounds.Y && point.Y < bounds.Y+bounds.Height
}
