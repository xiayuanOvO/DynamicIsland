package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestWindowCoversDisplay(t *testing.T) {
	display := mygo.Rectangle{X: -1920, Width: 1920, Height: 1080}
	for _, test := range []struct {
		name   string
		window mygo.Rectangle
		want   bool
	}{
		{"fullscreen", display, true},
		{"one-pixel rounding", mygo.Rectangle{X: -1919, Y: 1, Width: 1919, Height: 1079}, true},
		{"maximized within work area", mygo.Rectangle{X: -1920, Width: 1920, Height: 1032}, false},
		{"another monitor", mygo.Rectangle{X: 0, Width: 1920, Height: 1080}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := windowCoversDisplay(test.window, display); got != test.want {
				t.Fatalf("fullscreen = %v, want %v", got, test.want)
			}
		})
	}
}
