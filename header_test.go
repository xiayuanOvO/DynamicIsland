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
		{name: "playing with files", playing: true, files: true, wantWave: true},
		{name: "idle", wantFace: true},
		{name: "files", files: true},
		{name: "expanded", playing: true, expanded: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := island{files: &shelf.Shelf{}, expanded: test.expanded}
			if test.files {
				a.files.Items = []shelf.Item{{Name: "文件"}}
			}
			if test.playing {
				a.media.track = nowplaying.Track{Title: "曲目", Playing: true, Position: 83, Duration: 180, Rate: 1}
			}
			var open float32
			if test.expanded {
				open = 1
			}
			target := a.targetFrame()
			if !test.expanded && (target.width == target.height) != test.wantFace {
				t.Fatalf("idle circle has wrong dimensions: %+v", target)
			}
			view := ui.NewTester(func(c *ui.Context) {
				if a.media.track.Sampled.IsZero() {
					a.media.track.Sampled = c.Now()
				}
				a.header(c, open)
			}, int(target.width), int(target.height))
			if _, branded := view.Find("灵动岛"); branded {
				t.Fatal("header still displays the app name")
			}
			for _, label := range []string{"固定展开", "更多操作", "收起"} {
				if _, present := view.Find(label); present {
					t.Fatalf("header still displays %s", label)
				}
			}
			wave, hasWave := view.Find("播放律动")
			if hasWave != test.wantWave {
				t.Fatalf("wave visible = %v, want %v", hasWave, test.wantWave)
			}
			cover, hasCover := view.Find("专辑封面")
			elapsed, hasElapsed := view.Find("1:23")
			if hasCover != test.wantWave || hasElapsed != test.wantWave {
				t.Fatalf("cover visible = %v, elapsed visible = %v, want %v", hasCover, hasElapsed, test.wantWave)
			}
			if _, hasCount := view.Find("01"); hasCount != (test.files && !test.playing) {
				t.Fatalf("file count visible = %v", hasCount)
			}
			if test.wantWave {
				if target.width >= compactWidth || wave.W >= target.width/2 {
					t.Fatalf("playback capsule or waveform is too wide: %+v, %+v", target, wave)
				}
				if cover.X < 8 || wave.X-(cover.X+cover.W) < 8 || elapsed.X-(wave.X+wave.W) < 8 || target.width-(elapsed.X+elapsed.W) < 8 || wave.X+wave.W/2 != target.width/2 {
					t.Fatalf("expected cover, centered wave, and elapsed time within the capsule: %+v, %+v, %+v", cover, wave, elapsed)
				}
				view.Advance(time.Second)
				if _, updated := view.Find("1:24"); !updated {
					t.Fatal("elapsed time did not advance")
				}
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
		a.header(c, 0)
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

func TestCapsuleExpandsOnClickAndCollapsesAfterLeaving(t *testing.T) {
	a := island{files: &shelf.Shelf{}, width: compactMediaWidth, height: compactHeight}
	a.media.track = nowplaying.Track{Title: "测试曲目", Playing: true}
	view := ui.NewTester(func(c *ui.Context) {
		root := ui.Box(c).Fill()
		a.hover(c, root.Hovered())
		root.Children(func() { a.header(c, 0) })
	}, compactMediaWidth, compactHeight)
	view.Move(100, 26)
	view.Advance(time.Second)
	if a.expanded {
		t.Fatal("hover should leave the capsule collapsed")
	}
	view.Press(100, 26)
	view.Move(120, 26)
	view.Release(120, 26)
	if a.expanded {
		t.Fatal("dragging should not expand the capsule")
	}
	view.ClickAt(100, 26)
	if !a.expanded {
		t.Fatal("click did not expand the capsule")
	}
	view.Move(-1, -1)
	view.Advance(199 * time.Millisecond)
	if !a.expanded {
		t.Fatal("panel collapsed before the leave delay")
	}
	view.Advance(time.Millisecond)
	if a.expanded {
		t.Fatal("panel did not collapse after leaving")
	}
}

func TestExpandedMediaDoesNotReserveAnEmptyShelf(t *testing.T) {
	a := island{files: &shelf.Shelf{}, expanded: true}
	a.media.track = nowplaying.Track{Title: "测试曲目"}
	height, _ := a.panelHeights()
	view := ui.NewTester(func(c *ui.Context) { a.body(c, 1) }, panelWidth, height)
	for _, label := range []string{"暂存文件", "文件暂存区", "添加文件"} {
		if _, found := view.Find(label); found {
			t.Fatalf("empty shelf exposed %s", label)
		}
	}
	if _, found := view.Find("测试曲目"); !found || height >= 250 {
		t.Fatalf("expected a compact media-only panel, height %d", height)
	}
}

func TestIdleClickShowsAddWithoutShelf(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			a.header(c, 0)
			if a.expanded {
				a.body(c, 1)
			}
		})
	}, panelWidth, 180)
	view.ClickAt(panelWidth/2, compactHeight/2)
	if _, found := view.Find("添加文件"); !a.expanded || !found {
		t.Fatal("idle click did not reveal the add action")
	}
	for _, label := range []string{"暂存文件", "文件暂存区"} {
		if _, found := view.Find(label); found {
			t.Fatalf("empty shelf exposed %s", label)
		}
	}
}
