package island

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestIdleCapsuleStatus(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	a.idle.weather = idleWeather{condition: "局部多云伴有阵雨", temperature: -100, icon: weatherRainIcon}
	a.idle.forecastUntil = time.Now().Add(time.Minute)
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
	a.shownFrame.width = float32(width - 2*idleBatteryStep + 10)
	view.SetSize(int(a.shownFrame.width), idleCapsuleHeight)
	clock, hasClock = view.Find(view.Now().Format("15:04"))
	if !hasClock || clock.X+clock.W != a.shownFrame.width-idleCapsulePadding || !view.HasText("本机") || view.HasText("耳机 左耳") {
		t.Fatalf("the clock should keep its place while more batteries make room: %+v, %v", clock, view.Texts())
	}
	a.setExpanded(true)
	a.shownFrame = a.targetFrame()
	view.SetSize(int(a.shownFrame.width), int(a.shownFrame.height))
	view.Advance(2 * time.Minute)
	a.setExpanded(false)
	a.shownFrame = a.targetFrame()
	a.idle.reading = true
	a.battery.reading = true
	view.SetSize(width, idleCapsuleHeight)
	if !view.HasText("-100°C") || !view.HasText("本机") || !view.HasText(view.Now().Format("15:04")) {
		t.Fatalf("the capsule should show its cached status while refreshing after a mode change: %v", view.Texts())
	}
	a.idle.weather.temperature = 18
	a.idle.reading = false
	view.Frame()
	if !view.HasText("18°C") || view.HasText("-100°C") {
		t.Fatalf("the capsule should display the refreshed temperature: %v", view.Texts())
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

func TestIdleCapsuleDragDoesNotOpen(t *testing.T) {
	for _, test := range []struct {
		name     string
		releaseX float32
	}{
		{"final movement on release", 51},
		{"return to the starting point", 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := island{files: &shelf.Shelf{}}
			a.shownFrame = a.targetFrame()
			view := ui.NewTester(func(c *ui.Context) { a.header(c, 0) }, int(a.shownFrame.width), idleCapsuleHeight)
			view.Press(20, 20)
			view.Move(50, 20)
			view.Release(test.releaseX, 20)
			if a.expanded {
				t.Fatal("releasing a drag should leave the island collapsed")
			}
			view.ClickAt(test.releaseX, 20)
			if !a.expanded {
				t.Fatal("a separate click after a drag should expand the island")
			}
		})
	}
}

func TestFetchIdleWeatherIcons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		fmt.Fprintf(w, `{"current_condition":[{"temp_C":"-3","weatherCode":%q,%q:[{"value":%q}]}]}`,
			query.Get("code"), query.Get("field"), query.Get("condition"))
	}))
	t.Cleanup(server.Close)
	for _, test := range []struct {
		condition, code, field string
		icon                   *ui.SVG
	}{
		{"晴", "113", "lang_zh-cn", weatherSunIcon},
		{"Partly cloudy", "116", "weatherDesc", weatherPartlyCloudyIcon},
		{"多云", "119", "lang_zh-cn", weatherPartlyCloudyIcon},
		{"阴", "122", "lang_zh-cn", weatherCloudIcon},
		{"阵雨", "353", "lang_zh-cn", weatherRainIcon},
		{"暴雪", "230", "lang_zh-cn", weatherSnowIcon},
		{"雷阵雨", "386", "lang_zh-cn", weatherThunderIcon},
		{"冻雾", "260", "lang_zh-cn", weatherFogIcon},
		{"Smoky haze", "149", "weatherDesc", weatherFogIcon},
		{"局部多云伴有阵雨", "", "lang_xx", weatherRainIcon},
		{"未知天气", "999", "lang_zh-cn", weatherUnknownIcon},
	} {
		t.Run(test.condition, func(t *testing.T) {
			query := url.Values{"condition": {test.condition}, "code": {test.code}, "field": {test.field}}
			weather, err := fetchIdleWeather(context.Background(), server.URL+"?"+query.Encode())
			if err != nil {
				t.Fatal(err)
			}
			if weather.condition != test.condition || weather.temperature != -3 || weather.icon != test.icon || len(weather.rainDates) > 0 {
				t.Fatalf("unexpected weather or icon for %s: %+v", test.condition, weather)
			}
		})
	}
}

func TestFetchIdleWeatherTomorrowRain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"current_condition":[{"temp_C":"12","weatherCode":"113","lang_zh-cn":[{"value":"晴"}]}],
			"weather":[
				{"date":"2026-11-22","hourly":[{"weatherCode":"227"},{"weatherCode":"200"},{"weatherCode":"392"}]},
				{"date":"2026-11-21","hourly":[{"weatherCode":"113"},{"weatherCode":"386"}]},
				{"date":"2026-11-23","hourly":[{"weatherCode":"296"},{"weatherCode":"353"}]}
			]
		}`)
	}))
	t.Cleanup(server.Close)
	weather, err := fetchIdleWeather(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		day      int
		wantRain bool
	}{{19, false}, {20, true}, {21, false}, {22, true}} {
		now := time.Date(2026, time.November, test.day, 12, 0, 0, 0, time.Local)
		info := idleInfo{weather: weather, forecastUntil: now.Add(time.Hour)}
		if info.showsTomorrowRain(now) != test.wantRain {
			t.Errorf("November %d: want rain reminder = %t, got forecast %v", test.day, test.wantRain, weather.rainDates)
		}
	}
}

func TestIdleCapsuleRainReminder(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	noWeatherWidth := a.targetFrame().width
	a.idle.weather = idleWeather{condition: "晴", temperature: -100, icon: weatherSunIcon}
	baseWidth := a.targetFrame().width
	if baseWidth != noWeatherWidth+idleWeatherWidth+idleCapsuleGap {
		t.Fatalf("empty weather should not reserve capsule width: %.0f vs %.0f", noWeatherWidth, baseWidth)
	}

	view := ui.NewTester(func(c *ui.Context) { a.header(c, 0) }, capsuleAlertWidth, idleCapsuleHeight)
	now := view.Now()
	a.idle.weather.rainDates = []string{now.AddDate(0, 0, 1).Format(time.DateOnly)}
	a.idle.forecastUntil = now.Add(time.Hour)
	a.maybeTriggerRainAlert(now)
	view.Frame()

	// Enter animation completes after capsuleAlertEnter and shows standalone rain alert (without inline weather).
	view.Advance(capsuleAlertEnter)
	if !view.HasText("明日有雨") || !view.HasText("下雨动画") || view.HasText("-100°C") {
		t.Fatalf("tomorrow rain should display as standalone animated alert: %v", view.Texts())
	}

	// After alert duration elapses, normal idle capsule restores cleanly.
	view.Advance(capsuleAlertHold + capsuleAlertExit + 50*time.Millisecond)
	if view.HasText("明日有雨") || !view.HasText("-100°C") {
		t.Fatalf("idle capsule should restore after rain alert exits: %v", view.Texts())
	}
}

func TestIdleAlertExitDoesNotShrinkOrPopBatteries(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	a.idle.weather = idleWeather{condition: "晴", temperature: 22, icon: weatherSunIcon}
	a.battery.readings = []battery.Reading{
		{ID: "computer", Name: "本机", Kind: battery.Computer, Percent: 90},
		{ID: "earbud", Name: "耳机", Kind: battery.Headphones, Percent: 70},
		{ID: "mouse", Name: "鼠标", Kind: battery.Mouse, Percent: 12},
	}
	idleWidth := a.targetFrame().width
	if idleWidth <= capsuleAlertWidth {
		t.Fatalf("expected idle capsule with 3 batteries to exceed alert width, got %.0f", idleWidth)
	}

	now := time.Now()
	a.showCapsuleAlert(capsuleAlertTomorrowRain, battery.Reading{}, now)
	if alertWidth := a.targetFrame().width; alertWidth != idleWidth {
		t.Fatalf("alert in wide idle mode should keep idle width %.0f instead of shrinking to %.0f", idleWidth, alertWidth)
	}

	a.shownFrame = a.targetFrame()
	view := ui.NewTester(func(c *ui.Context) { a.header(c, 0) }, int(idleWidth), idleCapsuleHeight)
	a.showCapsuleAlert(capsuleAlertLowBattery, a.battery.readings[2], view.Now())
	view.Frame()
	// Advance into the exit crossfade: all 3 batteries should fade in together without popping.
	view.Advance(capsuleAlertEnter + capsuleAlertHold + capsuleAlertExit/2)
	for _, label := range []string{"本机", "耳机", "鼠标"} {
		if !view.HasText(label) {
			t.Fatalf("all batteries should remain visible during alert exit without re-expanding, missing %q in %v", label, view.Texts())
		}
	}
}

func TestIdleCapsuleLimitsBatteryCountAndTucksOnDock(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	base, _ := a.idleCapsuleMetrics(a.settings.idleItems())
	maxBatteries := (panelWidth - base) / idleBatteryStep
	for i := 0; i <= maxBatteries; i++ {
		a.battery.readings = append(a.battery.readings, battery.Reading{
			ID: fmt.Sprint(i), Name: fmt.Sprintf("设备 %d", i), Percent: 50 + i,
		})
	}
	a.shownFrame = a.targetFrame()
	if a.shownFrame.width <= compactHeight || a.shownFrame.width > panelWidth {
		t.Fatalf("idle capsule width = %.0f, want a capsule that fits within the panel width", a.shownFrame.width)
	}
	view := ui.NewTester(func(c *ui.Context) { a.header(c, a.shownFrame.open) }, int(a.shownFrame.width), idleCapsuleHeight)
	if !view.HasText(fmt.Sprintf("设备 %d", maxBatteries-1)) || view.HasText(fmt.Sprintf("设备 %d", maxBatteries)) {
		t.Fatalf("battery indicators should be limited to %d: %v", maxBatteries, view.Texts())
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
