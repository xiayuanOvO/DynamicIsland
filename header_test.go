package main

import (
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestCompactHeaderMatchesActivity(t *testing.T) {
	for _, test := range []struct {
		name     string
		playing  bool
		expanded bool
		files    bool
		wantWave bool
		wantFace bool
	}{
		{name: "playing", playing: true, wantWave: true},
		{name: "idle", wantFace: true},
		{name: "files", files: true},
		{name: "expanded", expanded: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := island{files: &shelf.Shelf{}, expanded: test.expanded}
			if test.files {
				a.files.Items = []shelf.Item{{Name: "文件"}}
			}
			if test.playing {
				a.media.track = nowplaying.Track{Title: "曲目", Playing: true}
			}
			var open float32
			if test.expanded {
				open = 1
			}
			target := a.targetFrame()
			if !test.expanded && (target.width == target.height) != test.wantFace {
				t.Fatalf("idle circle has wrong dimensions: %+v", target)
			}
			view := ui.NewTester(func(c *ui.Context) { a.header(c, open, open) }, int(target.width), int(target.height))
			if _, branded := view.Find("灵动岛"); branded {
				t.Fatal("header still displays the app name")
			}
			if _, hasWave := view.Find("播放律动"); hasWave != test.wantWave {
				t.Fatalf("wave visible = %v, want %v", hasWave, test.wantWave)
			}
			if rect, hasFace := view.Find("空闲表情"); hasFace != test.wantFace {
				t.Fatalf("face visible = %v, want %v", hasFace, test.wantFace)
			} else if hasFace && (rect.X+rect.W/2 != target.width/2 || rect.Y+rect.H/2 != target.height/2) {
				t.Fatalf("face is off-center: %+v", rect)
			}
		})
	}
}

func TestIdleEyesBlinkAndReturn(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	view := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		a.header(c, 0, 0)
	}, compactHeight, compactHeight)

	eyeHeight := func() (int, int) {
		frame := view.Image()
		top, bottom := compactHeight, 0
		for y := range compactHeight {
			for x := range compactHeight {
				if frame.RGBAAt(x, y).A > 30 {
					top, bottom = min(top, y), max(bottom, y)
				}
			}
		}
		return bottom - top + 1, top + bottom
	}
	open, center := eyeHeight()
	if open < 10 || center >= compactHeight-2 {
		t.Fatalf("expected open eyes above center: height %d, center %v", open, float64(center)/2)
	}
	delay := a.blinkAt.Sub(view.Now())
	if delay < 6*time.Second || delay > 11*time.Second {
		t.Fatalf("unexpected delay before blink: %v", delay)
	}
	view.Advance(delay + 72*time.Millisecond)
	closed, _ := eyeHeight()
	if closed < 1 || closed >= open/2 {
		t.Fatalf("eyes did not close: open %d, closed %d", open, closed)
	}
	view.Advance(150 * time.Millisecond)
	reopened, _ := eyeHeight()
	delay = a.blinkAt.Sub(view.Now())
	if reopened != open || delay < 5900*time.Millisecond || delay > 11*time.Second {
		t.Fatalf("eyes did not reopen and wait: height %d, delay %v", reopened, delay)
	}
}
