package island

import (
	"math"
	"testing"
	"time"

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
