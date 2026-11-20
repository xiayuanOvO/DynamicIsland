package island

import (
	"math"
	"testing"
	"time"

	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestTodoCardFollowsWindowHeight(t *testing.T) {
	a := island{
		expanded: true,
		files:    &shelf.Shelf{},
		todos:    todoComponent{items: []todoItem{{ID: "example", Title: "示例待办"}}},
	}
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
	for _, button := range []string{"查看待办事项", "返回岛面板"} {
		before, ok := view.Find("待办卡片")
		if !ok {
			t.Fatal("todo card is missing")
		}
		if err := view.Click(button); err != nil {
			t.Fatal(err)
		}
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
			card, ok := view.Find("待办卡片")
			if !ok {
				t.Fatal("todo card disappeared during the transition")
			}
			if delta := card.Y + card.H + 20 - a.shownFrame.height; math.Abs(float64(delta)) > 1 {
				t.Fatalf("%s: todo card and window heights differ by %.1f points", button, delta)
			}
			if a.todos.open && card.W < before.W || !a.todos.open && card.W > before.W {
				t.Fatalf("%s: todo card moved in the wrong direction from %.1f to %.1f", button, before.W, card.W)
			}
			if tick == 0 && (a.shownFrame.height == a.targetFrame().height || card.W == before.W) {
				t.Fatalf("%s: the first frame did not show an in-progress transition", button)
			}
			before = card
		}
		if a.shownFrame != a.targetFrame() {
			t.Fatalf("%s: the window did not settle at the target frame", button)
		}
	}
}
