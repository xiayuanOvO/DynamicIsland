package main

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTooltipTextVisibleOnGlass(t *testing.T) {
	const tip = "添加文件"
	view := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme)
		ui.Box(c.Key("toolbar")).Padding(40).Children(func() {
			iconButton(c.Key("add"), plusIcon, tip).Label("anchor")
		})
	}, 240, 160)
	view.Move(53, 53)
	view.Advance(time.Second)

	rect, ok := view.Find(tip)
	if !ok {
		t.Fatal("tooltip did not open")
	}
	frame := view.Image()
	bounds := frame.Bounds().Intersect(image.Rect(int(rect.X), int(rect.Y), int(rect.X+rect.W), int(rect.Y+rect.H)))
	for p := bounds.Min; p.Y < bounds.Max.Y; p.Y++ {
		for p.X = bounds.Min.X; p.X < bounds.Max.X; p.X++ {
			c := frame.RGBAAt(p.X, p.Y)
			if c.A > 240 && c.R > 180 && c.G > 180 && c.B > 180 {
				return
			}
		}
	}
	t.Fatal("tooltip contains no light text pixels")
}
