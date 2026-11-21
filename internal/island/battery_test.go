package island

import (
	"testing"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestBatteryRowInOverview(t *testing.T) {
	a := &island{expanded: true, files: &shelf.Shelf{}}
	a.settings.IdleItems = defaultIdleItems
	a.settings.IdleItems[2].Hidden = true
	withoutBattery := a.targetFrame()
	a.battery.readings = []battery.Reading{
		{ID: "computer", Name: "本机", Kind: battery.Computer, Percent: 100, Charging: true},
		{ID: "mouse", Name: "测试鼠标", Kind: battery.Mouse, Percent: battery.UnknownPercent},
		{ID: "earbuds-left", Name: "测试耳机", Kind: battery.Headphones, Part: battery.Left, Percent: 0},
	}
	a.shownFrame = a.targetFrame()
	if got := a.shownFrame.height - withoutBattery.height; got != batteryRowHeight+batteryRowGap {
		t.Fatalf("battery row added %.1f points to the overview", got)
	}
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
	}, panelWidth, int(a.shownFrame.height))
	for _, label := range []string{"100%", "—", "左 0%"} {
		if !view.HasText(label) {
			t.Errorf("missing battery text %q", label)
		}
	}
	for _, label := range []string{"本机", "测试鼠标 · 电量未知", "测试耳机 左耳"} {
		if bounds, ok := view.Find(label); !ok || bounds.H <= 0 {
			t.Errorf("missing battery indicator %q", label)
		}
	}
	a.todos.open = true
	a.shownFrame = a.targetFrame()
	view.SetSize(panelWidth, int(a.shownFrame.height))
	if view.HasText("100%") || !view.HasText("待办事项") {
		t.Fatal("the battery row should give way to the full todo panel")
	}
}
