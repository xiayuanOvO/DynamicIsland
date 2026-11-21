package island

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestIdleCapsuleStatus(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	a.idle.weather = idleWeather{condition: "局部多云伴有阵雨", temperature: -100, icon: weatherRainIcon}
	a.idle.weatherUntil = time.Now().Add(time.Minute)
	a.battery.readings = []battery.Reading{
		{ID: "computer", Name: "本机", Kind: battery.Computer, Percent: 84, Charging: true},
		{ID: "mouse", Name: "鼠标", Kind: battery.Mouse, Percent: battery.UnknownPercent},
		{ID: "left", Name: "耳机", Kind: battery.Headphones, Part: battery.Left, Percent: 100},
	}
	a.shownFrame = a.targetFrame()
	width := int(a.shownFrame.width)
	if a.shownFrame.height != idleCapsuleHeight || a.shownFrame.height >= compactHeight {
		t.Fatalf("idle capsule height = %.0f, want %d", a.shownFrame.height, idleCapsuleHeight)
	}
	view := ui.NewTester(func(c *ui.Context) { a.header(c, a.shownFrame.open) }, width, idleCapsuleHeight)
	weather, hasWeather := view.Find(a.idle.weather.condition)
	temp, hasTemp := view.Find("-100°C")
	device, hasDevice := view.Find("本机")
	leftEar, hasEar := view.Find("耳机 左耳")
	clock, hasClock := view.Find(view.Now().Format("15:04"))
	if !hasWeather || !hasTemp || !hasDevice || !hasEar || !hasClock || !view.HasText("鼠标 · 电量未知") {
		t.Fatalf("missing capsule status: %v", view.Texts())
	}
	if weather.W != 14 || weather.H != 14 || weather.X+weather.W > temp.X {
		t.Fatalf("weather icon should fit to the left of the temperature: icon %+v, temperature %+v", weather, temp)
	}
	if temp.X+temp.W > device.X-idleCapsuleGap || leftEar.X+leftEar.W > clock.X || clock.X+clock.W != float32(width-idleCapsulePadding) {
		t.Fatalf("clock should be to the right of weather and batteries: temperature %+v, battery %+v, left ear %+v, clock %+v", temp, device, leftEar, clock)
	}
	if temp.H != clock.H {
		t.Fatalf("temperature and clock should have the same text height: %.1f and %.1f", temp.H, clock.H)
	}
	for _, box := range []ui.Rect{weather, temp, device, leftEar, clock} {
		if math.Abs(float64(box.Y+box.H/2-idleCapsuleHeight/2)) > 1 {
			t.Fatalf("weather, batteries and clock should share one row: %+v", box)
		}
	}
	for _, label := range []string{"84%", "左 100%", "—"} {
		if _, shown := view.Find(label); shown {
			t.Errorf("the compact battery indicator should omit %q", label)
		}
	}
	a.shownFrame.width = idleCapsuleBaseWidth + idleBatteryStep + 10
	view.SetSize(int(a.shownFrame.width), idleCapsuleHeight)
	clock, hasClock = view.Find(view.Now().Format("15:04"))
	if !hasClock || clock.X+clock.W != a.shownFrame.width-idleCapsulePadding || !view.HasText("本机") || view.HasText("耳机 左耳") {
		t.Fatalf("the clock should keep its place while more batteries make room: %+v, %v", clock, view.Texts())
	}
	a.shownFrame.width = float32(width)
	view.SetSize(width, idleCapsuleHeight)
	view.Advance(2 * time.Minute)
	if view.HasText("-100°C") || !view.HasText(view.Now().Format("15:04")) || !view.HasText("本机") {
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

func TestFetchIdleWeatherIcons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Query().Get("weather"))
	}))
	t.Cleanup(server.Close)
	for _, test := range []struct {
		response string
		icon     *ui.SVG
	}{
		{"晴|-3°C|113", weatherSunIcon},
		{"Partly cloudy|-3°C|116", weatherPartlyCloudyIcon},
		{"多云|-3°C|119", weatherPartlyCloudyIcon},
		{"阴|-3°C|122", weatherCloudIcon},
		{"阵雨|-3°C|353", weatherRainIcon},
		{"暴雪|-3°C|230", weatherSnowIcon},
		{"雷阵雨|-3°C|386", weatherThunderIcon},
		{"冻雾|-3°C|260", weatherFogIcon},
		{"Smoky haze|-3°C|149", weatherFogIcon},
		{"局部多云伴有阵雨|-3°C", weatherRainIcon},
		{"未知天气|-3°C|999", weatherUnknownIcon},
	} {
		t.Run(test.response, func(t *testing.T) {
			weather, err := fetchIdleWeather(context.Background(), server.URL+"?weather="+url.QueryEscape(test.response))
			if err != nil {
				t.Fatal(err)
			}
			condition, _, _ := strings.Cut(test.response, "|")
			if weather.condition != condition || weather.temperature != -3 || weather.icon != test.icon {
				t.Fatalf("unexpected weather or icon for %s: %+v", test.response, weather)
			}
		})
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
	view := ui.NewTester(func(c *ui.Context) { a.header(c, a.shownFrame.open) }, int(a.shownFrame.width), idleCapsuleHeight)
	if !view.HasText(fmt.Sprintf("设备 %d", idleMaxBatteries-1)) || view.HasText(fmt.Sprintf("设备 %d", idleMaxBatteries)) {
		t.Fatalf("battery indicators should be limited to %d: %v", idleMaxBatteries, view.Texts())
	}

	a.dock.side = dockRight
	a.shownFrame = a.targetFrame()
	if a.shownFrame.width != compactHeight || a.shownFrame.height != compactHeight {
		t.Fatalf("docked capsule size = %.0fx%.0f, want %dx%d", a.shownFrame.width, a.shownFrame.height, compactHeight, compactHeight)
	}
	view.SetSize(compactHeight, compactHeight)
	if !view.HasText("空闲表情") || view.HasText("设备 0") || view.HasText(view.Now().Format("15:04")) {
		t.Fatalf("the tucked capsule should show the face alone: %v", view.Texts())
	}
}
