package island

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestIdleDisplaySettingsReorderHideAndSave(t *testing.T) {
	a := island{files: &shelf.Shelf{}, statePath: filepath.Join(t.TempDir(), "shelf.json")}
	a.battery.readings = []battery.Reading{{ID: "computer", Name: "本机", Kind: battery.Computer, Percent: 80}}
	a.idle.weather = idleWeather{condition: "晴", temperature: 19, icon: weatherSunIcon}
	a.idle.weatherUntil = time.Now().Add(time.Hour)
	originalWidth := a.targetFrame().width
	view := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
	face, hasFace := view.Find("表情")
	clock, hasClock := view.Find("时间")
	if !hasFace || !hasClock || face.Y >= clock.Y {
		t.Fatalf("settings should show the default order: %v", view.Texts())
	}
	view.Press(clock.X+2, clock.Y+clock.H/2)
	view.Move(face.X+2, face.Y)
	view.Release(face.X+2, face.Y)
	want := idleItemSettings{{ID: idleItemClock}, {ID: idleItemFace}, {ID: idleItemWeather}, {ID: idleItemBattery}}
	if got := a.settings.idleItems(); got != want {
		t.Fatalf("dragging time to the top gave order %+v, want %+v", got, want)
	}

	const batterySwitch = "显示设备电量"
	for range 10 {
		if view.Focused(batterySwitch) {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused(batterySwitch) {
		t.Fatal("the device battery switch is not reachable by Tab")
	}
	view.Key(0, ui.KeyUp)
	want[2], want[3] = want[3], want[2]
	if got := a.settings.idleItems(); got != want || !view.Focused(batterySwitch) {
		t.Fatalf("moving the focused battery switch up gave %+v or lost focus", got)
	}
	view.Key(0, ui.KeySpace)
	want[2].Hidden = true
	view.Advance(time.Second)
	saved, err := loadSettings(a.settingsPath())
	if err != nil || saved.idleItems() != want || a.settings.idleItems() != want {
		t.Fatalf("the display settings did not persist: %+v, %v", saved.idleItems(), err)
	}

	a.shownFrame = a.targetFrame()
	if a.shownFrame.width != originalWidth-idleBatteryStep {
		t.Fatalf("hiding the battery should shrink the window width, got %.0f", a.shownFrame.width)
	}
	bar := ui.NewTester(func(c *ui.Context) { a.header(c, 0) }, int(a.shownFrame.width), idleCapsuleHeight)
	clock, hasClock = bar.Find(bar.Now().Format("15:04"))
	face, hasFace = bar.Find("空闲表情")
	weather, hasWeather := bar.Find("晴")
	if !hasClock || !hasFace || !hasWeather || !(clock.X < face.X && face.X < weather.X) || bar.HasText("本机") {
		t.Fatalf("the capsule did not apply the display order or visibility: %v", bar.Texts())
	}
	if err := view.Click(batterySwitch); err != nil {
		t.Fatal(err)
	}
	if !a.settings.showsIdleItem(idleItemBattery) || a.targetFrame().width != originalWidth {
		t.Fatal("showing the battery should restore its slot")
	}
	batteryLabel, found := view.Find("设备电量")
	if !found {
		t.Fatal("the battery row is missing from settings")
	}
	view.RightClickAt(batteryLabel.X+2, batteryLabel.Y+batteryLabel.H/2)
	if err := view.ChooseMenuItem("下移"); err != nil {
		t.Fatal(err)
	}
	want[2].Hidden = false
	want[2], want[3] = want[3], want[2]
	if a.settings.idleItems() != want {
		t.Fatalf("moving through the context menu gave %+v, want %+v", a.settings.idleItems(), want)
	}
}

func TestIdleItemsLoadMissingAndUnknownEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, test := range []struct {
		json string
		want idleItemSettings
	}{
		{`{"hideLyrics":true}`, defaultIdleItems},
		{`{"idleItems":[{"id":"clock","hidden":true},{"id":"unknown"},{"id":"weather"},{"id":"clock"}]}`,
			idleItemSettings{{ID: idleItemClock, Hidden: true}, {ID: idleItemWeather}, {ID: idleItemFace}, {ID: idleItemBattery}}},
	} {
		if err := os.WriteFile(path, []byte(test.json), 0600); err != nil {
			t.Fatal(err)
		}
		saved, err := loadSettings(path)
		if err != nil || saved.idleItems() != test.want {
			t.Fatalf("loading %s gave %+v, %v", test.json, saved.idleItems(), err)
		}
	}
}

func TestIdleCapsuleRemainsClickableWithAllItemsHidden(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	a.settings.IdleItems = a.settings.idleItems()
	for i := range a.settings.IdleItems {
		a.settings.IdleItems[i].Hidden = true
	}
	a.shownFrame = a.targetFrame()
	if a.shownFrame.width != compactHeight {
		t.Fatalf("all hidden should use the minimum idle width, got %.0f", a.shownFrame.width)
	}
	bar := ui.NewTester(func(c *ui.Context) { a.header(c, 0) }, int(a.shownFrame.width), idleCapsuleHeight)
	if bar.HasText("空闲表情") || bar.HasText(bar.Now().Format("15:04")) {
		t.Fatalf("hidden items still show in the capsule: %v", bar.Texts())
	}
	bar.ClickAt(compactHeight/2, idleCapsuleHeight/2)
	if !a.expanded {
		t.Fatal("the empty capsule should still open the island")
	}
}
