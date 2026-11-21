package island

import (
	"image"
	"image/color"
	"runtime"

	"github.com/egoist/mygo/ui"
)

var grain = frostGrain()

func islandCornerRadius(height float32) float32 {
	return min(windowRadius, height/2)
}

func glassSurface(c *ui.Context, element ui.Element, vibrant, over, movingGlass bool) {
	backdrop := element
	if movingGlass {
		// The native mask can move before the next Metal frame arrives.
		// Paint past the current capsule so it can't expose an untinted band.
		backdrop = c.Root()
	}
	switch {
	case c.Theme().Dark:
		start, end := ui.Hex("#27282c"), ui.Hex(settingsBackgroundDark)
		if vibrant {
			start, end = start.Alpha(0.93), end.Alpha(0.91)
		}
		backdrop.Gradient(start, end, 155)
	case runtime.GOOS == "windows" && vibrant:
		backdrop.Gradient(white.Alpha(0.70), white.Alpha(0.62), 155)
	case vibrant:
		// The system blurs the desktop. A light wash preserves its colors.
		backdrop.Gradient(white.Alpha(0.20), white.Alpha(0.07), 155)
	case runtime.GOOS == "windows":
		backdrop.Gradient(ui.Hex("#eceef1"), ui.Hex("#bfc4cb"), 155)
	default:
		backdrop.Gradient(ui.Hex("#f2f3f5"), ui.Hex("#e7e9ed"), 155)
	}
	// Keep the same content inset when the native glass draws the rim.
	element.BorderWidth(0.75)
	if runtime.GOOS != "windows" || !vibrant {
		backdrop.Draw(paintFrost)
	}
	if movingGlass && !over {
		// The native glass rim follows the mask. A rim in the last Metal
		// frame can sit a refresh behind it and look like a second outline.
		return
	}
	edge, middle, bottom, shine := white.Alpha(0.76), white.Alpha(0.24), white.Alpha(0.54), white.Alpha(0.48)
	if c.Theme().Dark {
		rim := ui.Hex("#55585f")
		edge, middle, bottom, shine = rim, rim.Alpha(0.4), rim.Alpha(0.7), rim.Alpha(0.18)
	}
	if over {
		edge = ui.RGB(207, 222, 242)
		if c.Theme().Dark {
			edge = c.Theme().Accent
		}
	}
	element.BorderColor(edge).BorderLinearGradient(ui.LinearGradient{
		Angle: 155, Stops: []ui.GradientStop{
			{Color: edge, At: 0},
			{Color: middle, At: 0.48},
			{Color: bottom, At: 1},
		},
	}).InsetShadow(0, 1, 1, 0, shine).
		InsetShadow(0, -1, 2, 0, ink.Alpha(0.035))
}

func paintFrost(p *ui.Painter, r ui.Rect) {
	radius := islandCornerRadius(r.H)
	light := white.Alpha(0.18)
	if p.Theme().Dark {
		light = white.Alpha(0.03)
	}
	p.Clip(r, radius, func() {
		p.FillRadialGradient(r, ui.RadialGradient{
			Center: ui.Point{X: 0.12, Y: 0.02}, Radius: r.W * 0.9,
			Stops: []ui.GradientStop{
				{Color: light, At: 0},
				{Color: ui.Transparent, At: 1},
			},
		}, radius)
		// A small stationary texture avoids a perfectly flat digital surface.
		for y := r.Y; y < r.Y+r.H; y += 128 {
			for x := r.X; x < r.X+r.W; x += 128 {
				p.Image(grain, ui.Rect{X: x, Y: y, W: 128, H: 128}, ui.Contain)
			}
		}
	})
}

func frostGrain() *ui.Bitmap {
	texture := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	seed := uint32(0x6d2b79f5)
	for y := range 128 {
		for x := range 128 {
			seed ^= seed << 13
			seed ^= seed >> 17
			seed ^= seed << 5
			value := uint8((seed & 1) * 255)
			texture.SetNRGBA(x, y, color.NRGBA{R: value, G: value, B: value, A: uint8(1 + seed%3)})
		}
	}
	return ui.NewBitmap(texture)
}
