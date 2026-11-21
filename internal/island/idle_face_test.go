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
	eye := color.RGBA{48, 48, 48, 255}
	background := color.RGBA{240, 240, 240, 255}
	for _, test := range []struct {
		name          string
		mood          idleMood
		blink         bool
		filled, clear []image.Point
	}{
		{"neutral", moodNeutral, false,
			[]image.Point{{17, 14}, {33, 14}, {14, 17}, {14, 23}, {30, 17}, {30, 23}, {17, 26}, {33, 26}},
			[]image.Point{{17, 13}, {33, 13}, {14, 14}, {30, 14}, {25, 20}, {17, 27}, {33, 27}}},
		{"sleepy", moodSleepy, false, []image.Point{{17, 23}, {33, 23}}, []image.Point{{17, 20}, {33, 20}}},
		{"bored", moodBored, false, []image.Point{{16, 17}, {32, 20}}, []image.Point{{32, 17}}},
		{"happy", moodHappy, false, []image.Point{{17, 16}, {33, 16}}, []image.Point{{17, 20}, {33, 20}}},
		{"looking", moodLooking, false, []image.Point{{19, 20}, {35, 20}}, []image.Point{{15, 20}}},
		{"blink", moodNeutral, true, []image.Point{{17, 20}, {33, 20}}, []image.Point{{17, 18}, {33, 22}}},
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
				if got := img.RGBAAt(p.X*scale, p.Y*scale); got != eye {
					t.Errorf("eye at %v = %v, want %v", p, got, eye)
				}
			}
			for _, p := range test.clear {
				if got := img.RGBAAt(p.X*scale, p.Y*scale); got != background {
					t.Errorf("background at %v = %v, want %v", p, got, background)
				}
			}
			for y := 28 * scale; y < 34*scale; y++ {
				for x := 19 * scale; x < 31*scale; x++ {
					if got := img.RGBAAt(x, y); got != background {
						t.Fatalf("unexpected mark below the eyes at (%d, %d): %v", x, y, got)
					}
				}
			}
		})
	}
}
