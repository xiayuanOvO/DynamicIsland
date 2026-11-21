package island

import (
	"fmt"
	"testing"
	"time"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestIdleCapsuleStatus(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	a.idle.weather = idleWeather{condition: "局部多云伴有阵雨", temperature: -12}
	a.idle.weatherUntil = time.Now().Add(time.Minute)
	a.battery.readings = []battery.Reading{
		{ID: "computer", Name: "本机", Kind: battery.Computer, Percent: 84, Charging: true},
		{ID: "mouse", Name: "鼠标", Kind: battery.Mouse, Percent: battery.UnknownPercent},
		{ID: "left", Name: "耳机", Kind: battery.Headphones, Part: battery.Left, Percent: 100},
	}
	a.shownFrame = a.targetFrame()
	width := int(a.shownFrame.width)
	view := ui.NewTester(func(c *ui.Context) { a.header(c, a.shownFrame.open) }, width, compactHeight)
	temp, hasTemp := view.Find("-12°C")
	device, hasDevice := view.Find("本机")
	leftEar, hasEar := view.Find("耳机 左耳")
	if !hasTemp || !hasDevice || !hasEar || !view.HasText("鼠标 · 电量未知") || !view.HasText(view.Now().Format("15:04")) {
		t.Fatalf("missing capsule status: %v", view.Texts())
	}
	if temp.X+temp.W > device.X || leftEar.X+leftEar.W > float32(width) {
		t.Fatalf("weather or battery indicators overlap or overflow: temperature %+v, battery %+v, left ear %+v", temp, device, leftEar)
	}
	for _, label := range []string{"84%", "左 100%", "—"} {
		if _, shown := view.Find(label); shown {
			t.Errorf("the compact battery indicator should omit %q", label)
		}
	}
	view.Advance(2 * time.Minute)
	if view.HasText("-12°C") || !view.HasText(view.Now().Format("15:04")) || !view.HasText("本机") {
		t.Fatalf("expired weather should give way to the clock and battery: %v", view.Texts())
	}

	view.ClickAt(device.X+device.W/2, device.Y+device.H/2)
	if !a.expanded {
		t.Fatal("clicking a battery indicator should expand the island")
	}
	a.setExpanded(false)
	view.Key(0, ui.KeyTab)
	if !view.Focused("展开灵动岛") {
		t.Fatal("the capsule should be the keyboard target")
	}
	view.Key(0, ui.KeyEnter)
	if !a.expanded {
		t.Fatal("pressing Enter on the capsule should expand the island")
	}
}

func TestIdleCapsuleLimitsBatteryCountAndTucksOnDock(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	for i := 0; i <= idleMaxBatteries; i++ {
		a.battery.readings = append(a.battery.readings, battery.Reading{
			ID: fmt.Sprint(i), Name: fmt.Sprintf("设备 %d", i), Percent: 50 + i,
		})
	}
	a.shownFrame = a.targetFrame()
	if a.shownFrame.width <= compactHeight || a.shownFrame.width > panelWidth {
		t.Fatalf("idle capsule width = %.0f, want a capsule that fits within the panel width", a.shownFrame.width)
	}
	view := ui.NewTester(func(c *ui.Context) { a.header(c, a.shownFrame.open) }, int(a.shownFrame.width), compactHeight)
	if !view.HasText(fmt.Sprintf("设备 %d", idleMaxBatteries-1)) || view.HasText(fmt.Sprintf("设备 %d", idleMaxBatteries)) {
		t.Fatalf("battery indicators should be limited to %d: %v", idleMaxBatteries, view.Texts())
	}

	a.dock.side = dockRight
	a.shownFrame = a.targetFrame()
	if a.shownFrame.width != compactHeight {
		t.Fatalf("docked capsule width = %.0f, want %d", a.shownFrame.width, compactHeight)
	}
	view.SetSize(compactHeight, compactHeight)
	if !view.HasText("空闲表情") || view.HasText("设备 0") || view.HasText(view.Now().Format("15:04")) {
		t.Fatalf("the tucked capsule should show the face alone: %v", view.Texts())
	}
}
