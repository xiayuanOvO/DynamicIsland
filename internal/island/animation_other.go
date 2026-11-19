//go:build !darwin

package island

import "github.com/egoist/mygo"

func startWindowDisplayLink(*island) func() { return nil }

func setAnimatedWindowBounds(win *mygo.Window, bounds mygo.Rectangle) { win.SetBounds(bounds) }

func startTransitionCanvas(*mygo.Window) transitionCanvas { return transitionCanvas{} }
