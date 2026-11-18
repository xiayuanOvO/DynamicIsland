package main

import (
	"bytes"
	"testing"
	"time"

	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestIdleMoodsRenderDistinctExpressions(t *testing.T) {
	var rendered [][]byte
	for _, mood := range []idleMood{moodNeutral, moodSleepy, moodBored, moodHappy, moodLooking} {
		a := island{files: &shelf.Shelf{}}
		a.face.mood, a.face.moodAt = mood, time.Now().Add(-time.Second)
		a.face.blinkAt = time.Now().Add(time.Hour)
		if mood == moodNeutral {
			a.face.moodAt = time.Now().Add(time.Hour)
		}
		view := ui.NewTester(func(c *ui.Context) {
			c.Root().Background(ui.Transparent)
			a.header(c, 0)
		}, compactHeight, compactHeight)
		pixels := view.Image().Pix
		for _, previous := range rendered {
			if bytes.Equal(pixels, previous) {
				t.Fatalf("mood %v matches an earlier expression", mood)
			}
		}
		rendered = append(rendered, bytes.Clone(pixels))
	}
}

func TestIdleEyesEaseTowardTheCursor(t *testing.T) {
	now := time.Now()
	var face idleFaceState
	face.lookAt(now, ui.Point{X: 120, Y: -60})
	first := face.gaze
	face.lookAt(now.Add(70*time.Millisecond), ui.Point{X: 120, Y: -60})
	second := face.gaze
	if !(first.X > 0 && second.X > first.X && second.X <= 1 && second.Y < first.Y && second.Y >= -1) {
		t.Fatalf("eyes did not ease up and right: %+v -> %+v", first, second)
	}
	face.lookAt(now.Add(350*time.Millisecond), ui.Point{X: -120, Y: 0})
	if face.gaze.X >= 0 || face.gaze.X < -1 {
		t.Fatalf("eyes did not turn left: %+v", face.gaze)
	}
}
