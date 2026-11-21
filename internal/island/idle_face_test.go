package island

import (
	"image"
	"image/color"
	"testing"
	"time"

	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestIdleFaceEyes(t *testing.T) {
	const scale = 4
	const dx = idleCapsulePadding + idleFaceWidth/2 - compactHeight/2
	const dy = (idleCapsuleHeight - compactHeight) / 2
	eye := color.RGBA{48, 48, 48, 255}
	background := color.RGBA{240, 240, 240, 255}
	for _, test := range []struct {
		name          string
		mood          idleMood
		blink         bool
		filled, clear []image.Point
	}{
		{"neutral", moodNeutral, false, []image.Point{{19, 16}, {19, 22}, {33, 22}, {33, 28}}, []image.Point{{15, 16}, {25, 22}, {19, 15}, {33, 29}}},
		{"sleepy", moodSleepy, false, []image.Point{{19, 25}, {33, 25}}, []image.Point{{19, 22}, {33, 22}}},
		{"bored", moodBored, false, []image.Point{{18, 19}, {32, 23}}, []image.Point{{32, 19}}},
		{"happy", moodHappy, false, []image.Point{{19, 18}, {33, 18}}, []image.Point{{19, 22}, {33, 22}}},
		{"looking", moodLooking, false, []image.Point{{25, 22}, {39, 22}}, []image.Point{{16, 22}}},
		{"blink", moodNeutral, true, []image.Point{{19, 22}, {33, 22}}, []image.Point{{19, 20}, {33, 24}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := island{files: &shelf.Shelf{}}
			a.shownFrame = a.targetFrame()
			img := ui.Render(func(c *ui.Context) {
				c.Root().Background(ui.RGB(240, 240, 240))
				now := c.Now()
				a.face = idleFaceState{
					mood: test.mood, moodAt: now.Add(-time.Second),
					blinkAt: now.Add(time.Second), gaze: ui.Point{X: 1},
				}
				if test.blink {
					a.face.blinkAt = now.Add(-72 * time.Millisecond)
				}
				a.header(c, 0)
			}, int(a.shownFrame.width), idleCapsuleHeight, scale)

			for _, p := range test.filled {
				if got := img.RGBAAt((p.X+dx)*scale, (p.Y+dy)*scale); got != eye {
					t.Errorf("eye at %v = %v, want %v", p, got, eye)
				}
			}
			for _, p := range test.clear {
				if got := img.RGBAAt((p.X+dx)*scale, (p.Y+dy)*scale); got != background {
					t.Errorf("background at %v = %v, want %v", p, got, background)
				}
			}
			for y := (31 + dy) * scale; y < (39+dy)*scale; y++ {
				for x := (20 + dx) * scale; x < (32+dx)*scale; x++ {
					if got := img.RGBAAt(x, y); got != background {
						t.Fatalf("unexpected mark below the eyes at (%d, %d): %v", x, y, got)
					}
				}
			}
		})
	}
}
