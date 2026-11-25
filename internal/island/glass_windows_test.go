package island

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWindowsGlassSurface(t *testing.T) {
	for _, test := range []struct {
		name     string
		backdrop bool
	}{
		{name: "system blur", backdrop: true},
		{name: "solid fallback"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const width, height = 200, compactHeight
			img := ui.Render(func(c *ui.Context) {
				c.Root().Background(ui.Transparent)
				surface := ui.Box(c).Fill().Radius(windowRadius).Clip()
				glassSurface(c, surface, test.backdrop, false)
			}, width, height, 1)
			alpha := img.RGBAAt(width/2, height/2).A
			if test.backdrop {
				if alpha < 150 || alpha > 190 {
					t.Fatalf("tinted blur center alpha = %d, want 150–190", alpha)
				}
			} else if alpha != 255 {
				t.Fatalf("fallback center alpha = %d, want 255", alpha)
			}
			if corner := img.RGBAAt(0, 0).A; corner != 0 {
				t.Fatalf("rounded corner alpha = %d, want 0", corner)
			}
		})
	}
}
