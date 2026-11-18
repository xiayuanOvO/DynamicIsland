package main

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type dockSide int

const (
	dockLeft  dockSide = -1
	dockNone  dockSide = 0
	dockRight dockSide = 1
	dockTuck           = compactHeight / 2
)

type edgeDock struct {
	side    dockSide
	display mygo.Display
}

type capsulePress struct {
	point    mygo.Point
	origin   mygo.Point
	moved    bool
	released bool
}

func (a *island) capsuleInput(ev ui.InputEvent) bool {
	if ev.Button != 0 {
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		a.press = &capsulePress{point: mygo.Point{X: int(ev.X), Y: int(ev.Y)}}
		if a.win != nil {
			a.press.point = mygo.Screen.CursorScreenPoint()
			bounds := a.win.Bounds()
			a.press.origin = mygo.Point{X: bounds.X, Y: bounds.Y}
		}
		return true
	case ui.InputPointerMove, ui.InputPointerUp:
		press := a.press
		if press == nil {
			return false
		}
		point := mygo.Point{X: int(ev.X), Y: int(ev.Y)}
		if a.win != nil {
			point = mygo.Screen.CursorScreenPoint()
		}
		dx, dy := point.X-press.point.X, point.Y-press.point.Y
		press.moved = press.moved || dx*dx+dy*dy >= 25
		if ev.Kind == ui.InputPointerUp {
			press.released = true
		} else if press.moved && a.win != nil {
			a.win.SetPosition(press.origin.X+dx, press.origin.Y+dy)
		}
		return true
	}
	return false
}

func dockSideFor(window, display mygo.Rectangle) dockSide {
	const distance = 22
	if display.Width <= 0 {
		return dockNone
	}
	left := window.X - display.X
	right := (display.X + display.Width) - (window.X + window.Width)
	if left <= distance && left < right {
		return dockLeft
	}
	if right <= distance {
		return dockRight
	}
	return dockNone
}

func (a *island) windowBounds(width, height int) mygo.Rectangle {
	bounds := mygo.Rectangle{X: a.centerX - width/2, Y: a.top, Width: width, Height: height}
	if a.dock.side == dockNone {
		return bounds
	}
	display, work := a.dock.display.Bounds, a.dock.display.WorkArea
	tuck := max(0, dockTuck-max(0, height-compactHeight)/2)
	bounds.X = work.X - (work.X-display.X+dockTuck)*tuck/dockTuck
	if a.dock.side == dockRight {
		displayRight, workRight := display.X+display.Width, work.X+work.Width
		bounds.X = workRight - width + (displayRight-workRight+dockTuck)*tuck/dockTuck
	}
	bounds.Y = min(max(bounds.Y, work.Y), max(work.Y, work.Y+work.Height-height))
	return bounds
}

func (a *island) windowMoved() {
	if a.resizing {
		return
	}
	bounds := a.win.Bounds()
	expected := a.windowBounds(bounds.Width, bounds.Height)
	if bounds.X == expected.X && bounds.Y == expected.Y {
		return
	}
	a.dock = edgeDock{}
	a.centerX, a.top = bounds.X+bounds.Width/2, bounds.Y
	if !a.moving && (runtime.GOOS == "darwin" || runtime.GOOS == "windows") {
		a.moving = true
		a.animation.pause(time.Now())
		a.leaveSince = time.Time{}
	}
	a.win.Invalidate()
}

func (a *island) finishMove(c *ui.Context) {
	if !a.moving {
		return
	}
	if (a.press != nil && !a.press.released) || primaryMouseDown() {
		c.After(32 * time.Millisecond)
		return
	}
	a.moving = false
	a.animation.resume(c.Now())
	bounds := a.win.Bounds()
	display := mygo.Screen.DisplayMatching(bounds)
	if side := dockSideFor(bounds, display.Bounds); side != dockNone {
		a.dock = edgeDock{side: side, display: display}
		a.setExpanded(false)
		a.resize(a.width, a.height)
	}
}
