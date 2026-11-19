package island

import (
	"time"

	"github.com/egoist/mygo"
)

func (a *island) watchFullscreen() {
	done := make(chan struct{})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { close(done) })
	check := func() {
		active := foregroundFullscreen()
		if a.fullscreen == active {
			return
		}
		a.fullscreen = active
		if active {
			a.hideWindows()
		} else if !a.hiddenByUser {
			a.media.refreshAt = time.Time{}
			a.win.ShowInactive()
			a.win.Invalidate()
		}
	}
	check()
	go func() {
		tick := time.NewTicker(400 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				mygo.RunOnMain(check)
			case <-done:
				return
			}
		}
	}()
}

func windowCoversDisplay(window, display mygo.Rectangle) bool {
	const tolerance = 2
	left := window.X - display.X
	top := window.Y - display.Y
	right := window.X + window.Width - display.X - display.Width
	bottom := window.Y + window.Height - display.Y - display.Height
	return display.Width > 0 && display.Height > 0 &&
		min(left, top, right, bottom) >= -tolerance &&
		max(left, top, right, bottom) <= tolerance
}
