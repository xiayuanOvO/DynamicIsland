package island

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
)

const supportsFullscreenHiding = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

func (a *island) watchFullscreen() {
	if !supportsFullscreenHiding {
		return
	}
	done := make(chan struct{})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { close(done) })
	a.syncFullscreenVisibility()
	go func() {
		tick := time.NewTicker(400 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				mygo.RunOnMain(a.syncFullscreenVisibility)
			case <-done:
				return
			}
		}
	}()
}

func (a *island) syncFullscreenVisibility() {
	hidden := a.settings.HideInFullscreen && foregroundFullscreen()
	if a.hiddenForFullscreen == hidden {
		return
	}
	a.hiddenForFullscreen = hidden
	if hidden {
		a.hideWindows()
	} else if !a.hiddenByUser {
		a.win.ShowInactive()
		a.win.Invalidate()
	}
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
