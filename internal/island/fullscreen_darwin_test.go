package island

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestWindowCoversMacDisplay(t *testing.T) {
	screen := mygo.Rectangle{Width: 1470, Height: 956}
	fullscreenWorkArea := mygo.Rectangle{Y: 32, Width: 1470, Height: 924}
	fullscreenWindow := mygo.Rectangle{Y: 37, Width: 1470, Height: 919}
	windowedWorkArea := mygo.Rectangle{Y: 38, Width: 1470, Height: 846}
	hiddenDockWorkArea := mygo.Rectangle{Y: 38, Width: 1470, Height: 914}
	for _, tc := range []struct {
		name     string
		workArea mygo.Rectangle
		window   mygo.Rectangle
		want     bool
	}{
		{"full display", screen, screen, true},
		{"fullscreen below the camera housing", fullscreenWorkArea, fullscreenWindow, true},
		{"fullscreen toolbar strip", fullscreenWorkArea, mygo.Rectangle{Y: 37, Width: 1470, Height: 44}, false},
		{"window below the reserved area", fullscreenWorkArea, mygo.Rectangle{Y: 80, Width: 1470, Height: 876}, false},
		{"maximized with the Dock visible", windowedWorkArea, windowedWorkArea, false},
		{"maximized with the Dock hidden", hiddenDockWorkArea, hiddenDockWorkArea, false},
		{"maximized on a display without the Dock", fullscreenWorkArea, fullscreenWorkArea, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			display := mygo.Display{Bounds: screen, WorkArea: tc.workArea}
			if got := windowCoversMacDisplay(tc.window, display); got != tc.want {
				t.Fatalf("windowCoversMacDisplay(%+v, %+v) = %v, want %v", tc.window, display, got, tc.want)
			}
		})
	}
}
