package main

import (
	"testing"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestDockAndLyricsStayOnDisplay(t *testing.T) {
	display := mygo.Display{
		Bounds:   mygo.Rectangle{X: -1440, Y: 18, Width: 1440, Height: 900},
		WorkArea: mygo.Rectangle{X: -1410, Y: 42, Width: 1380, Height: 876},
	}
	for _, release := range []struct {
		x    int
		side dockSide
	}{
		{-1440 + 16, dockLeft},
		{-compactMediaWidth - 16, dockRight},
		{-900, dockNone},
	} {
		bounds := mygo.Rectangle{X: release.x, Y: 42, Width: compactMediaWidth, Height: compactHeight}
		if side := dockSideFor(bounds, display.Bounds); side != release.side {
			t.Fatalf("dock side for %+v = %v, want %v", bounds, side, release.side)
		}
		if release.side == dockNone {
			continue
		}
		a := island{files: &shelf.Shelf{}, dock: edgeDock{side: release.side, display: display}, top: bounds.Y}
		a.media.track = nowplaying.Track{Title: "测试曲目", Playing: true}
		frame := a.targetFrame()
		if frame.width != compactHeight || frame.height != compactHeight {
			t.Fatalf("expected a compact circle while docked: %+v", frame)
		}
		capsule := a.windowBounds(int(frame.width), int(frame.height))
		panel := a.windowBounds(panelWidth, 440)
		lyric := lyricBounds(capsule, display.WorkArea, release.side)
		if panel.X < display.WorkArea.X || panel.X+panel.Width > display.WorkArea.X+display.WorkArea.Width ||
			lyric.X < display.WorkArea.X || lyric.X+lyric.Width > display.WorkArea.X+display.WorkArea.Width ||
			lyric.Y < display.WorkArea.Y || lyric.Y+lyric.Height > display.WorkArea.Y+display.WorkArea.Height {
			t.Fatalf("panel or lyrics outside display: %+v, %+v", panel, lyric)
		}
		switch release.side {
		case dockLeft:
			if capsule.X+capsule.Width/2 != display.Bounds.X || lyric.X <= capsule.X+capsule.Width {
				t.Fatalf("left dock and lyric placement: %+v, %+v", capsule, lyric)
			}
		case dockRight:
			if capsule.X+capsule.Width/2 != display.Bounds.X+display.Bounds.Width || lyric.X+lyric.Width >= capsule.X {
				t.Fatalf("right dock and lyric placement: %+v, %+v", capsule, lyric)
			}
		}
	}
}

func TestDockedPlaybackHeaderFitsVisibleHalf(t *testing.T) {
	for _, side := range []dockSide{dockLeft, dockRight} {
		a := island{files: &shelf.Shelf{}, dock: edgeDock{side: side}}
		a.media.track = nowplaying.Track{Title: "测试曲目", Playing: true, Position: 12}
		view := ui.NewTester(func(c *ui.Context) {
			c.Root().Background(ui.Transparent)
			a.header(c, 0)
		}, compactHeight, compactHeight)
		if _, found := view.Find("专辑封面"); found {
			t.Fatal("album cover should be part of the full capsule")
		}
		if _, found := view.Find("0:12"); found {
			t.Fatal("elapsed time should be part of the full capsule")
		}
		frame := view.Image()
		visible := 0
		for y := range compactHeight {
			for x := range compactHeight {
				if frame.RGBAAt(x, y).A == 0 {
					continue
				}
				if (x < compactHeight/2) != (side == dockRight) {
					t.Fatalf("waveform at (%d,%d) is outside the visible half for side %v", x, y, side)
				}
				visible++
			}
		}
		if visible == 0 {
			t.Fatal("missing compact waveform")
		}
	}
}

func TestDockedFileGridFitsShortDisplay(t *testing.T) {
	a := island{
		files:    &shelf.Shelf{Items: make([]shelf.Item, 4)},
		expanded: true,
		dock:     edgeDock{side: dockLeft, display: mygo.Display{WorkArea: mygo.Rectangle{Width: 960, Height: 510}}},
	}
	a.media.track = nowplaying.Track{Title: "测试曲目"}
	if height := a.targetFrame().height; height > 510 {
		t.Fatalf("docked panel exceeds work area: %v", height)
	}
}
