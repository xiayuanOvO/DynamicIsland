package island

import (
	"fmt"
	"math"
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestPanelCardsFollowWindowHeight(t *testing.T) {
	for _, test := range []struct {
		name, label, button string
		right               bool
	}{
		{"todos", "待办卡片", "查看待办事项", false},
		{"clipboard", "剪贴板卡片", "查看剪贴板历史", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &island{files: &shelf.Shelf{}}
			if test.right {
				a = newTestClipboard(t, true, "示例记录")
			} else {
				a.todos.items = []todoItem{{ID: "example", Title: "示例待办"}}
			}
			a.expanded = true
			overview := a.targetFrame()
			a.shownFrame = overview
			a.animation = islandAnimation{from: overview, to: overview, started: time.Now()}
			view := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Clip().Children(func() {
					ui.Box(c).Height(panelHeaderHeight).Shrink(0)
					a.body(c, a.shownFrame)
				})
			}, panelWidth, int(overview.height))

			now := a.animation.started
			initial, ok := view.Find(test.label)
			if !ok {
				t.Fatal("card is missing")
			}
			before := initial
			for _, button := range []string{test.button, "返回岛面板"} {
				if err := view.Click(button); err != nil {
					t.Fatal(err)
				}
				opening := button == test.button
				if a.animation.started.After(now) {
					now = a.animation.started
				}
				a.animation.retarget(a.targetFrame(), now)
				for tick := range int(islandResizeDuration/windowFrameInterval) + 1 {
					elapsed := windowFrameInterval
					if tick == 4 {
						elapsed *= 4 // Simulate a missed display refresh.
					}
					now = now.Add(elapsed)
					a.shownFrame = a.animation.advance(now).aligned()
					view.Advance(elapsed)
					view.SetSize(panelWidth, int(a.shownFrame.height))
					card, ok := view.Find(test.label)
					if !ok {
						t.Fatal("card disappeared during the transition")
					}
					if delta := card.Y + card.H + 20 - a.shownFrame.height; math.Abs(float64(delta)) > 1 {
						t.Fatalf("%s: card and window heights differ by %.1f points", button, delta)
					}
					edge := card.X - initial.X
					if test.right {
						edge += card.W - initial.W
					}
					if math.Abs(float64(edge)) > 1 {
						t.Fatalf("%s: the card moved away from its original edge by %.1f points", button, edge)
					}
					if opening && card.W < before.W || !opening && card.W > before.W {
						t.Fatalf("%s: card width moved in the wrong direction from %.1f to %.1f", button, before.W, card.W)
					}
					if tick == 0 && (a.shownFrame.height == a.targetFrame().height || card.W == before.W) {
						t.Fatalf("%s: the first frame did not show an in-progress transition", button)
					}
					before = card
				}
				if a.shownFrame != a.targetFrame() {
					t.Fatalf("%s: the window did not settle at the target frame", button)
				}
				if opening && test.right && !view.Focused("搜索剪贴板历史") {
					t.Fatal("the expanded clipboard panel should focus search")
				}
			}
		})
	}
}

func TestClipboardOpeningFadesOverviewBeforeRemovingIt(t *testing.T) {
	a := newTestClipboard(t, true, "示例记录")
	for i := range cardColumns * maxCardRows {
		name := fmt.Sprintf("示例文件 %d.txt", i+1)
		a.files.Items = append(a.files.Items, shelf.Item{Path: name, Name: name})
	}
	a.media.track = nowplaying.Track{Title: "示例曲目"}
	a.todos.items = []todoItem{{ID: "example", Title: "示例待办"}}
	a.status = "示例状态"
	a.expanded = true
	overview := a.targetFrame()
	a.shownFrame = overview
	now := time.Now()
	a.animation = islandAnimation{from: overview, to: overview, started: now}
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
	}, panelWidth, int(overview.height))
	overviewLabels := []string{"示例曲目", "示例待办", "示例文件 6.txt", "拖出全部", "示例状态"}
	initial := make([]ui.Rect, len(overviewLabels))
	for i, label := range overviewLabels {
		var ok bool
		initial[i], ok = view.Find(label)
		if !ok || initial[i].H == 0 {
			t.Fatalf("%q should be fully visible in the overview", label)
		}
	}
	if err := view.Click("查看剪贴板历史"); err != nil {
		t.Fatal(err)
	}
	for _, elapsed := range []time.Duration{35 * time.Millisecond, 60 * time.Millisecond, 75 * time.Millisecond, 100 * time.Millisecond} {
		frame, _ := a.animation.sample(a.animation.started.Add(elapsed))
		a.shownFrame = frame.aligned()
		view.SetSize(panelWidth, int(a.shownFrame.height))
		fading := frame.clipboard < panelCardPreviewEnd
		for i, label := range overviewLabels {
			box, ok := view.Find(label)
			if fading && (!ok || box.W != initial[i].W || box.H != initial[i].H) {
				t.Fatalf("%v: %q should stay whole while fading; changed from %+v to %+v", elapsed, label, initial[i], box)
			}
			if !fading && ok {
				t.Fatalf("%v: %q should be gone when the history appears", elapsed, label)
			}
		}
	}
}

func TestPanelOpeningKeepsMediaCardGeometry(t *testing.T) {
	for _, style := range []int{mediaCardDefault, mediaCardLargeCover} {
		a := newTestClipboard(t, true, "示例记录")
		a.settings.MediaCardStyle = style
		a.media.track = nowplaying.Track{Title: "示例曲目", Artist: "示例艺人", Duration: 190, Previous: true, Toggle: true}
		a.expanded = true
		a.shownFrame = a.targetFrame()
		now := time.Now()
		a.animation = islandAnimation{from: a.shownFrame, to: a.shownFrame, started: now}
		view := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Clip().Children(func() {
				ui.Box(c).Height(panelHeaderHeight).Shrink(0)
				a.body(c, a.shownFrame)
			})
		}, panelWidth, int(a.shownFrame.height))
		labels := []string{"示例曲目", "示例艺人", "上一首", "3:10"}
		bounds := make([]ui.Rect, len(labels))
		for i, label := range labels {
			box, ok := view.Find(label)
			if !ok || box.H <= 0 {
				t.Fatalf("style %d: could not find %q before opening", style, label)
			}
			bounds[i] = box
		}
		if err := view.Click("查看剪贴板历史"); err != nil {
			t.Fatal(err)
		}
		for _, elapsed := range []time.Duration{35 * time.Millisecond, 60 * time.Millisecond, 75 * time.Millisecond} {
			frame, _ := a.animation.sample(a.animation.started.Add(elapsed))
			a.shownFrame = frame.aligned()
			view.SetSize(panelWidth, int(a.shownFrame.height))
			for i, label := range labels {
				if box, ok := view.Find(label); !ok || box != bounds[i] {
					t.Fatalf("style %d at %v: %q changed from %+v to %+v", style, elapsed, label, bounds[i], box)
				}
			}
		}
		frame, _ := a.animation.sample(a.animation.started.Add(100 * time.Millisecond))
		a.shownFrame = frame.aligned()
		view.SetSize(panelWidth, int(a.shownFrame.height))
		if !view.HasText("示例记录") || view.HasText("示例曲目") {
			t.Fatalf("style %d: the music card should finish fading out as the history appears", style)
		}
	}
}
