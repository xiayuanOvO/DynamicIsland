package island

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestWindowCoversWindowsDisplay(t *testing.T) {
	display := mygo.Rectangle{X: -2560, Width: 2560, Height: 1440}
	workArea := mygo.Rectangle{X: -2560, Width: 2560, Height: 1368}
	for _, tc := range []struct {
		name   string
		window mygo.Rectangle
		style  uintptr
		want   bool
	}{
		{"borderless fullscreen", display, 0, true},
		{"fullscreen entered from maximized", display, wsMaximize, true},
		{"maximized with a caption and auto-hide taskbar", display, wsMaximize | wsCaption, false},
		{"maximized with a custom title bar and resize frame", display, wsMaximize | wsThickFrame, false},
		{"borderless maximized within work area", workArea, wsMaximize, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowCoversWindowsDisplay(tc.window, display, tc.style); got != tc.want {
				t.Errorf("windowCoversWindowsDisplay(%+v, %+v, %#x) = %v, want %v", tc.window, display, tc.style, got, tc.want)
			}
		})
	}
}
