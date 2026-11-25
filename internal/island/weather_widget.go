package island

import (
	"fmt"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
)

type weatherSlotItem struct {
	at       time.Time
	label    string
	icon     *ui.SVG
	temp     string
	sunEvent bool
}

func (a *island) weatherWidget(c *ui.Context, size widgetSize, width, height float32) {
	switch size {
	case widgetSizeSmall:
		a.weatherSmallCard(c, width, height)
	case widgetSizeLarge:
		a.weatherLargeCard(c, width, height)
	default:
		a.weatherMediumCard(c, width, height)
	}
}

func weatherCardColors(icon *ui.SVG, dark bool) (top, bottom ui.Color) {
	switch icon {
	case weatherSunIcon:
		if dark {
			return ui.RGB(38, 102, 174), ui.RGB(62, 128, 198)
		}
		return ui.RGB(52, 128, 208), ui.RGB(84, 158, 228)
	case weatherRainIcon, weatherThunderIcon:
		if dark {
			return ui.RGB(56, 68, 86), ui.RGB(74, 88, 108)
		}
		return ui.RGB(76, 90, 110), ui.RGB(98, 114, 134)
	case weatherSnowIcon:
		if dark {
			return ui.RGB(84, 112, 142), ui.RGB(106, 134, 162)
		}
		return ui.RGB(108, 140, 172), ui.RGB(134, 164, 192)
	default:
		if dark {
			return ui.RGB(92, 104, 120), ui.RGB(112, 124, 140)
		}
		return ui.RGB(122, 136, 154), ui.RGB(142, 156, 172)
	}
}

func (a *island) weatherLocationLabel() (name string, located bool) {
	if a.settings.WeatherCity.Name != "" {
		return a.settings.WeatherCity.Name, false
	}
	if a.idle.cache.CityName != "" {
		return a.idle.cache.CityName, true
	}
	return "当前位置", true
}

func (a *island) weatherTodayRange(now time.Time) (high, low int, ok bool) {
	today := now.Format(time.DateOnly)
	for _, day := range a.idle.cache.Days {
		if day.Date == today {
			if day.HasMin {
				return day.Temperature, day.MinTemp, true
			}
			return day.Temperature, a.idle.weather.temperature, true
		}
	}
	if len(a.idle.cache.Days) > 0 {
		day := a.idle.cache.Days[0]
		if day.HasMin {
			return day.Temperature, day.MinTemp, true
		}
	}
	return 0, 0, false
}

func parseWeatherLocalTime(raw string, loc *time.Location) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (a *island) weatherHourlySlots(now time.Time, maxSlots int) []weatherSlotItem {
	loc := now.Location()
	currentHour := now.Truncate(time.Hour)
	var hours []weatherSlotItem
	for _, h := range a.idle.cache.Hourly {
		t, ok := parseWeatherLocalTime(h.Time, loc)
		if !ok || t.Before(currentHour) {
			continue
		}
		hours = append(hours, weatherSlotItem{
			at:    t,
			label: fmt.Sprintf("%d时", t.Hour()),
			icon:  weatherIconForSource(a.idle.cache.Source, h.Code, ""),
			temp:  fmt.Sprintf("%d°", h.Temperature),
		})
		if len(hours) >= maxSlots {
			break
		}
	}
	if len(hours) == 0 {
		return nil
	}
	windowStart := hours[0].at
	windowEnd := hours[len(hours)-1].at.Add(time.Hour)
	var sunEvents []weatherSlotItem
	for _, day := range a.idle.cache.Days {
		for _, pair := range []struct {
			raw  string
			icon *ui.SVG
		}{
			{day.Sunrise, sunriseIcon},
			{day.Sunset, sunsetIcon},
		} {
			if pair.raw == "" {
				continue
			}
			t, ok := parseWeatherLocalTime(pair.raw, loc)
			if !ok || !t.After(windowStart) || !t.Before(windowEnd) {
				continue
			}
			temp := fmt.Sprintf("%d°", a.idle.weather.temperature)
			for _, h := range hours {
				if !h.at.After(t) {
					temp = h.temp
				}
			}
			sunEvents = append(sunEvents, weatherSlotItem{
				at:       t,
				label:    t.Format("15:04"),
				icon:     pair.icon,
				temp:     temp,
				sunEvent: true,
			})
		}
	}
	if len(sunEvents) == 0 {
		return hours
	}
	combined := make([]weatherSlotItem, 0, len(hours)+len(sunEvents))
	combined = append(combined, hours...)
	combined = append(combined, sunEvents...)
	sort.SliceStable(combined, func(i, j int) bool {
		return combined[i].at.Before(combined[j].at)
	})
	if len(combined) > maxSlots {
		combined = combined[:maxSlots]
	}
	return combined
}

func weatherWeekdayLabel(dateStr string, now time.Time) string {
	t, err := time.ParseInLocation(time.DateOnly, dateStr, now.Location())
	if err != nil {
		return dateStr
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if t.Equal(today) {
		return "今天"
	}
	names := [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return names[t.Weekday()]
}

func (a *island) weatherSmallCard(c *ui.Context, width, height float32) {
	w := a.idle.weather
	topColor, bottomColor := weatherCardColors(w.icon, c.Theme().Dark)
	cityName, located := a.weatherLocationLabel()
	high, low, hasRange := a.weatherTodayRange(c.Now())

	card := ui.Column(c.Key("weather-small")).Size(width, height).Shrink(0).Radius(13).Clip().
		Padding(10, 12).Justify(ui.SpaceBetween).
		Gradient(topColor, bottomColor, 180).Border(0.75, white.Alpha(0.22)).
		Label("天气卡片")
	card.Children(func() {
		ui.Row(c.Key("weather-top")).FillWidth().AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Text(c, cityName).SingleLine().Ellipsis("…").FontSize(12.5).FontWeight(650).TextColor(white)
			if located {
				ui.Icon(c, locationArrowIcon).Size(10, 10).Shrink(0).TextColor(white.Alpha(0.92))
			}
			ui.Box(c).Grow(1)
			if w.icon != nil {
				ui.Icon(c, w.icon).Size(17, 17).Shrink(0).TextColor(white)
			}
		})
		ui.Text(c, fmt.Sprintf("%d°", w.temperature)).FontSize(34).FontWeight(350).FontFeatures("tnum").
			SingleLine().TextColor(white)
		ui.Row(c.Key("weather-bottom")).FillWidth().AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Text(c, w.condition).SingleLine().Ellipsis("…").FontSize(11).FontWeight(550).TextColor(white.Alpha(0.92))
			ui.Box(c).Grow(1)
			if hasRange {
				ui.Text(c, fmt.Sprintf("最高%d° 最低%d°", high, low)).SingleLine().
					FontSize(10.5).FontWeight(500).FontFeatures("tnum").TextColor(white.Alpha(0.86))
			}
		})
	})
}

func (a *island) weatherHeader(c *ui.Context) {
	w := a.idle.weather
	cityName, located := a.weatherLocationLabel()
	high, low, hasRange := a.weatherTodayRange(c.Now())

	ui.Row(c.Key("weather-header")).FillWidth().Height(52).Shrink(0).AlignItems(ui.Center).Children(func() {
		ui.Column(c.Key("weather-left")).Gap(1).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
				ui.Text(c, cityName).SingleLine().Ellipsis("…").FontSize(13).FontWeight(650).TextColor(white)
				if located {
					ui.Icon(c, locationArrowIcon).Size(11, 11).Shrink(0).TextColor(white.Alpha(0.92))
				}
			})
			ui.Text(c, fmt.Sprintf("%d°", w.temperature)).FontSize(30).FontWeight(350).FontFeatures("tnum").
				SingleLine().TextColor(white)
		})
		ui.Box(c).Grow(1)
		ui.Column(c.Key("weather-right")).AlignItems(ui.End).Gap(2).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
				if w.icon != nil {
					ui.Icon(c, w.icon).Size(16, 16).Shrink(0).TextColor(white)
				}
				ui.Text(c, w.condition).SingleLine().FontSize(12).FontWeight(550).TextColor(white.Alpha(0.95))
			})
			if hasRange {
				ui.Text(c, fmt.Sprintf("最高%d° 最低%d°", high, low)).SingleLine().
					FontSize(11.5).FontWeight(500).FontFeatures("tnum").TextColor(white.Alpha(0.9))
			}
		})
	})
}

func (a *island) weatherHourlyRow(c *ui.Context, slots []weatherSlotItem) {
	if len(slots) == 0 {
		return
	}
	ui.Row(c.Key("weather-hourly")).FillWidth().Height(46).Shrink(0).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		for i, slot := range slots {
			ui.Column(c.Key(i)).Width(52).Shrink(0).Gap(3).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, slot.label).SingleLine().FontSize(10.5).FontWeight(550).FontFeatures("tnum").TextColor(white.Alpha(0.8))
				iconColor := white
				if slot.sunEvent {
					iconColor = ui.RGB(255, 210, 64)
				}
				if slot.icon != nil {
					ui.Icon(c, slot.icon).Size(15, 15).Shrink(0).TextColor(iconColor)
				}
				ui.Text(c, slot.temp).SingleLine().FontSize(11.5).FontWeight(600).FontFeatures("tnum").TextColor(white)
			})
		}
	})
}

func (a *island) weatherMediumCard(c *ui.Context, width, height float32) {
	w := a.idle.weather
	topColor, bottomColor := weatherCardColors(w.icon, c.Theme().Dark)
	slots := a.weatherHourlySlots(c.Now(), 6)

	card := ui.Column(c.Key("weather-medium")).Size(width, height).Shrink(0).Radius(13).Clip().
		Padding(9, 14).Justify(ui.SpaceBetween).
		Gradient(topColor, bottomColor, 180).Border(0.75, white.Alpha(0.22)).
		Label("天气卡片")
	card.Children(func() {
		a.weatherHeader(c)
		if len(slots) > 0 {
			a.weatherHourlyRow(c, slots)
		}
	})
}

func (a *island) weatherLargeCard(c *ui.Context, width, height float32) {
	w := a.idle.weather
	topColor, bottomColor := weatherCardColors(w.icon, c.Theme().Dark)
	now := c.Now()
	slots := a.weatherHourlySlots(now, 6)

	// Pick up to 4 upcoming days starting from today.
	today := now.Format(time.DateOnly)
	var days []weatherCacheDay
	for _, day := range a.idle.cache.Days {
		if day.Date >= today {
			days = append(days, day)
			if len(days) == 4 {
				break
			}
		}
	}
	if len(days) == 0 && len(a.idle.cache.Days) > 0 {
		days = a.idle.cache.Days[:min(4, len(a.idle.cache.Days))]
	}
	minAll, maxAll := 0, 0
	for i, d := range days {
		lo := d.Temperature
		if d.HasMin {
			lo = d.MinTemp
		}
		if i == 0 || lo < minAll {
			minAll = lo
		}
		if i == 0 || d.Temperature > maxAll {
			maxAll = d.Temperature
		}
	}
	tempSpan := max(1, maxAll-minAll)

	card := ui.Column(c.Key("weather-large")).Size(width, height).Shrink(0).Radius(13).Clip().
		Padding(10, 14).Gap(6).
		Gradient(topColor, bottomColor, 180).Border(0.75, white.Alpha(0.22)).
		Label("天气卡片")
	card.Children(func() {
		a.weatherHeader(c)
		if len(slots) > 0 {
			a.weatherHourlyRow(c, slots)
		}
		if len(days) > 0 {
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(white.Alpha(0.18))
			ui.Column(c.Key("weather-daily")).FillWidth().Grow(1).Justify(ui.SpaceAround).Children(func() {
				for _, day := range days {
					lo := day.Temperature
					if day.HasMin {
						lo = day.MinTemp
					}
					hi := day.Temperature
					startFrac := float32(lo-minAll) / float32(tempSpan)
					endFrac := float32(hi-minAll) / float32(tempSpan)
					icon := weatherIconForSource(a.idle.cache.Source, day.Code, day.Condition)
					ui.Row(c.Key(day.Date)).FillWidth().Height(24).Shrink(0).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c, weatherWeekdayLabel(day.Date, now)).Width(36).Shrink(0).SingleLine().
							FontSize(12).FontWeight(600).TextColor(white)
						if icon != nil {
							ui.Icon(c, icon).Size(15, 15).Shrink(0).TextColor(white)
						}
						ui.Text(c, fmt.Sprintf("%d°", lo)).Width(28).Shrink(0).TextAlign(ui.End).SingleLine().
							FontSize(11.5).FontWeight(550).FontFeatures("tnum").TextColor(white.Alpha(0.76))
						ui.Box(c).Grow(1).Height(4).Draw(func(p *ui.Painter, r ui.Rect) {
							p.Fill(r, white.Alpha(0.20), 2)
							bx := r.X + r.W*startFrac
							bw := max(6, r.W*(endFrac-startFrac))
							if bx+bw > r.X+r.W {
								bx = r.X + r.W - bw
							}
							p.Fill(ui.Rect{X: bx, Y: r.Y, W: bw, H: r.H}, ui.RGB(255, 210, 96), 2)
						})
						ui.Text(c, fmt.Sprintf("%d°", hi)).Width(28).Shrink(0).TextAlign(ui.End).SingleLine().
							FontSize(11.5).FontWeight(600).FontFeatures("tnum").TextColor(white)
					})
				}
			})
		}
	})
}
