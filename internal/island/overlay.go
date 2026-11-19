package island

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *island) placeOverlay(window **mygo.Window, bounds mygo.Rectangle, title string, view func(*ui.Context)) {
	win := *window
	if bounds.Width <= 0 || bounds.Height <= 0 {
		if win != nil {
			win.Hide()
		}
		return
	}
	if win == nil {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: title, Parent: a.win, Content: ui.View(view),
			X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
			Hidden: true, Frameless: true, Transparent: true, Panel: true,
			AlwaysOnTop: true, SkipTaskbar: true, IgnoreMouseEvents: true,
			DisableResize: true, DisableShadow: true, BackgroundColor: "#00000000",
		})
		*window = win
	} else {
		if win.Bounds() != bounds {
			win.SetBounds(bounds)
		}
		win.Invalidate()
	}
	if !win.IsVisible() {
		win.ShowInactive()
	}
}
