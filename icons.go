package main

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
	inboxIcon   = svg(`<path d="M4 4h16l2 11v5H2v-5L4 4Z"/><path d="M2 15h6l2 3h4l2-3h6M12 6v7m-3-3 3 3 3-3"/>`)
	fileIcon    = svg(`<path d="M14 2H5v20h14V7l-5-5Z"/><path d="M14 2v6h5M8 13h8m-8 4h5"/>`)
	folderIcon  = svg(`<path d="M3 6V4h7l2 3h9v13H3V6Z"/><path d="M3 8h18"/>`)
	imageIcon   = svg(`<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8" cy="8" r="1.5"/><path d="m3 17 5-5 4 4 4-6 5 7"/>`)
	archiveIcon = svg(`<rect x="4" y="3" width="16" height="18" rx="2"/><path d="M10 3v3h4v3h-4v3h4v3h-4v3h4"/>`)
	mediaIcon   = svg(`<rect x="3" y="3" width="18" height="18" rx="4"/><path d="m10 8 6 4-6 4V8Z"/>`)
	plusIcon    = svg(`<path d="M12 5v14M5 12h14"/>`)
	closeIcon   = svg(`<path d="m7 7 10 10M7 17 17 7"/>`)
	sendIcon    = svg(`<path d="M13 4h7v7m0-7L10 14M8 5H4v15h15v-5"/>`)
	copyIcon    = svg(`<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V3H3v13h5"/>`)
	trashIcon   = svg(`<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>`)
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
