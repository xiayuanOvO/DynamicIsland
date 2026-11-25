package island

import (
	"image"
	"image/color"
	"testing"
	"time"

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
			[]image.Point{{19, 14}, {31, 14}, {16, 17}, {16, 23}, {28, 17}, {28, 23}, {21, 20}, {33, 20}, {19, 26}, {31, 26}},
			[]image.Point{{19, 13}, {31, 13}, {16, 14}, {28, 14}, {15, 20}, {22, 20}, {27, 20}, {34, 20}, {19, 27}, {31, 27}}},
		{"sleepy", moodSleepy, false, []image.Point{{19, 23}, {31, 23}}, []image.Point{{19, 20}, {31, 20}}},
		{"bored", moodBored, false, []image.Point{{18, 17}, {30, 20}}, []image.Point{{30, 17}}},
		{"happy", moodHappy, false, []image.Point{{19, 16}, {31, 16}}, []image.Point{{19, 20}, {31, 20}}},
		{"looking", moodLooking, false, []image.Point{{21, 20}, {33, 20}}, []image.Point{{17, 20}}},
		{"blink", moodNeutral, true, []image.Point{{19, 20}, {31, 20}}, []image.Point{{19, 18}, {31, 22}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := island{}
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

func TestDarkIdleCapsuleHasLightEyes(t *testing.T) {
	a := island{settings: appSettings{Theme: themeDark}}
	a.shownFrame = a.targetFrame()
	const scale = 4
	img := ui.Render(func(c *ui.Context) {
		c.SetTheme(islandThemes[a.settings.Theme])
		capsule := ui.Column(c).Fill().Radius(idleCapsuleHeight / 2).Clip()
		glassSurface(c, capsule, false, false)
		capsule.Children(func() { a.header(c, 0) })
	}, int(a.shownFrame.width), idleCapsuleHeight, scale)
	background := img.RGBAAt(25*scale, 20*scale)
	lightest := max(background.R, background.G, background.B)
	darkest := min(background.R, background.G, background.B)
	if background.A != 255 || lightest >= 54 || lightest-darkest > 10 || background.R > background.B {
		t.Fatalf("the capsule should have a neutral dark gray background, got %v", background)
	}
	for _, x := range []int{19, 31} {
		if eye := img.RGBAAt(x*scale, 20*scale); eye.R < 230 || eye.G < 230 || eye.B < 230 {
			t.Errorf("the eye at x=%d should be light, got %v", x, eye)
		}
	}
}
