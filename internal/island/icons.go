package island

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/egoist/mygo/ui"
)

func svg(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

var (
	backIcon     = svg(`<path d="m11 6-6 6 6 6m-6-6h14"/>`)
	plusIcon     = svg(`<path d="M12 5v14M5 12h14"/>`)
	todoIcon     = svg(`<rect x="3" y="3" width="18" height="18" rx="5"/><path d="m7 12 3 3 7-7"/>`)
	imageIcon    = svg(`<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8" cy="8" r="1.5"/><path d="m3 17 5-5 4 4 4-6 5 7"/>`)
	mediaIcon    = svg(`<rect x="3" y="3" width="18" height="18" rx="4"/><path d="m10 8 6 4-6 4V8Z"/>`)
	copyIcon     = svg(`<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V3H3v13h5"/>`)
	trashIcon    = svg(`<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>`)
	settingsIcon = svg(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h.01a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51h.01a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v.01a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>`)
)

var clipboardIcon = svg(`<rect x="8" y="3" width="8" height="4" rx="1.4"/><path d="M8 5H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2M8 12h8m-8 4h6"/>`)

const weatherCloudShape = `<path d="M6 15a3.5 3.5 0 0 1 0-7h.8a5 5 0 0 1 9.5 1h1.2a3 3 0 0 1 0 6Z"/>`

var (
	weatherSunIcon          = svg(`<circle cx="12" cy="12" r="4.5"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>`)
	weatherPartlyCloudyIcon = svg(`<path d="M4.7 11.6A4.5 4.5 0 1 1 13 8M8 2v1.5M2.6 4.5l1 1M1.5 9H3m10-5 1-1"/>` +
		`<path d="M8 20a3.2 3.2 0 0 1 0-6.4h.6a4.7 4.7 0 0 1 8.9-1.2h.5A3.8 3.8 0 0 1 18 20Z"/>`)
	weatherCloudIcon   = svg(`<g transform="translate(0 2)">` + weatherCloudShape + `</g>`)
	weatherRainIcon    = svg(weatherCloudShape + `<path d="m8 18-1.5 3m6-3L11 21m6-3-1.5 3"/>`)
	weatherSnowIcon    = svg(`<path d="M12 2v20M3.3 7l17.4 10M3.3 17 20.7 7m-12-3 3.3 2 3.3-2m-6.6 16 3.3-2 3.3 2M3.6 11l3.5-2-.2-4M17.1 19l-.2-4 3.5-2M3.6 13l3.5 2-.2 4M17.1 5l-.2 4 3.5 2"/>`)
	weatherThunderIcon = svg(weatherCloudShape + `<path d="m13 15-4 4h4l-2 3"/>`)
	weatherFogIcon     = svg(weatherCloudShape + `<path d="M4 18h16M6 21h12"/>`)
	weatherUnknownIcon = svg(`<path d="M10 14V5a2 2 0 0 1 4 0v9a4 4 0 1 1-4 0Zm2-3v6"/><circle cx="12" cy="17.5" r="1"/>`)
)

func trayIcon() []byte {
	icon := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 6; y < 26; y++ {
		for x := 1; x < 31; x++ {
			cx := min(max(x, 11), 20)
			dx, dy := x-cx, y-16
			if dx*dx+dy*dy < 100 {
				icon.SetNRGBA(x, y, color.NRGBA{R: 48, G: 52, B: 59, A: 255})
			}
		}
	}
	for y := 14; y < 18; y++ {
		for x := 8; x < 14; x++ {
			icon.SetNRGBA(x, y, color.NRGBA{})
		}
		for x := 20; x < 24; x++ {
			icon.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, icon); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
