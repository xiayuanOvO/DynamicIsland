//go:build !darwin

package island

import "github.com/egoist/mygo"

func startWindowDisplayLink(*island) func() { return nil }

func setIslandWindowBounds(win *mygo.Window, bounds mygo.Rectangle, _ bool) { win.SetBounds(bounds) }

func startTransitionCanvas(*mygo.Window) transitionCanvas { return transitionCanvas{} }

func setIslandWindowCornerRadius(*mygo.Window, float32) {}
