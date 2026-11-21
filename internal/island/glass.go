package island

import (
	"image"
	"image/color"
	"runtime"

	"github.com/egoist/mygo/ui"
)

var (
	ink    = ui.Hex("#30343b")
	muted  = ui.Hex("#767b84")
	accent = ui.Hex("#657b96")
	white  = ui.RGB(255, 255, 255)
	theme  = islandTheme()
	grain  = frostGrain()
)

func islandTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Text, t.TextMuted, t.Accent = ink, muted, accent
	t.InactiveAccent = accent
	t.AccentText = white
	t.Background = ui.Transparent
	t.InverseText = white
	t.Surface = white.Alpha(0.32)
	t.SurfaceHover = white.Alpha(0.52)
	t.SurfacePressed = ink.Alpha(0.07)
	t.Border = ink.Alpha(0.10)
	t.Focus = accent.Alpha(0.4)
	t.Scrollbar = ink.Alpha(0.18)
	t.FontSize, t.Radius = 13, 10
	return t
}

func islandCornerRadius(height float32) float32 {
	return min(windowRadius, height/2)
}

func glassSurface(element ui.Element, vibrant, over, movingGlass bool) {
	switch {
	case runtime.GOOS == "windows" && vibrant:
		element.Gradient(white.Alpha(0.70), white.Alpha(0.62), 155)
	case vibrant:
		// The system blurs the desktop. A light wash preserves its colors.
		element.Gradient(white.Alpha(0.20), white.Alpha(0.07), 155)
	case runtime.GOOS == "windows":
		element.Gradient(ui.Hex("#eceef1"), ui.Hex("#bfc4cb"), 155)
	default:
		element.Gradient(ui.Hex("#f2f3f5"), ui.Hex("#e7e9ed"), 155)
	}
	// Keep the same content inset when the native glass draws the rim.
	element.BorderWidth(0.75)
	if runtime.GOOS != "windows" || !vibrant {
		element.Draw(paintFrost)
	}
	if movingGlass && !over {
		// The native glass rim follows the mask. A rim in the last Metal
		// frame can sit a refresh behind it and look like a second outline.
		return
	}
	edge := white.Alpha(0.76)
	if over {
		edge = ui.RGB(207, 222, 242)
	}
	element.BorderColor(edge).BorderLinearGradient(ui.LinearGradient{
		Angle: 155, Stops: []ui.GradientStop{
			{Color: edge, At: 0},
			{Color: white.Alpha(0.24), At: 0.48},
			{Color: white.Alpha(0.54), At: 1},
		},
	}).InsetShadow(0, 1, 1, 0, white.Alpha(0.48)).
		InsetShadow(0, -1, 2, 0, ink.Alpha(0.035))
}

func paintFrost(p *ui.Painter, r ui.Rect) {
	radius := islandCornerRadius(r.H)
	p.Clip(r, radius, func() {
		p.FillRadialGradient(r, ui.RadialGradient{
			Center: ui.Point{X: 0.12, Y: 0.02}, Radius: r.W * 0.9,
			Stops: []ui.GradientStop{
				{Color: white.Alpha(0.18), At: 0},
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
