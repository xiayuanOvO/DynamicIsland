package island

import (
	"testing"
	"time"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/nowplaying"
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

func TestLowBatteryAlertNotifiesOnceAndShowsPlaybackBanner(t *testing.T) {
	var notified []battery.Reading
	a := &island{
		files:    &shelf.Shelf{},
		settings: appSettings{LowBatteryAlert: true},
		media: islandMedia{
			track: nowplaying.Track{Title: "夜行列车", Playing: true, Position: 12, Duration: 200, Rate: 1, Sampled: time.Now()},
		},
		battery: islandBattery{
			readings: []battery.Reading{
				{ID: "computer", Name: "本机", Kind: battery.Computer, Percent: 10},
				{ID: "mouse-1", Name: "ROG Keris", Kind: battery.Mouse, Percent: 5},
			},
			notifyLow: func(r battery.Reading) { notified = append(notified, r) },
		},
	}

	view := ui.NewTester(func(c *ui.Context) { a.header(c, 0) }, compactMediaWidth, idleCapsuleHeight)
	now := view.Now()
	a.checkLowBattery(now)
	a.checkLowBattery(now.Add(30 * time.Second))
	if len(notified) != 1 || notified[0].ID != "mouse-1" {
		t.Fatalf("expected one low-battery notification for bluetooth mouse only, got %+v", notified)
	}

	view.Advance(capsuleAlertEnter)
	if !view.HasText("ROG Keris") || !view.HasText("5%") || !view.HasText("低电量动画") {
		t.Fatalf("playback capsule should show standalone animated low-battery alert: %v", view.Texts())
	}

	view.Advance(capsuleAlertHold + capsuleAlertExit + 50*time.Millisecond)
	if view.HasText("ROG Keris") || !view.HasText("播放律动") {
		t.Fatalf("playback capsule should restore wave after alert exits: %v", view.Texts())
	}

	// Charging resets the alert so a subsequent drop notifies again.
	a.battery.readings[1].Charging = true
	a.checkLowBattery(view.Now())
	a.battery.readings[1].Charging = false
	a.checkLowBattery(view.Now())
	if len(notified) != 2 {
		t.Fatalf("expected notification to re-arm after charging, got %d", len(notified))
	}
}
