package island

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	idleWeatherBase     = "https://wttr.in/"
	idleWeatherQuery    = "?format=j1&lang=zh-cn&m"
	idleWeatherEndpoint = idleWeatherBase + idleWeatherQuery

	idleCapsulePadding    = 10
	idleCapsuleGap        = 2
	idleCapsuleHeight     = 40
	idleFaceWidth         = 30
	idleWeatherWidth      = 66
	idleRainReminderWidth = 60
	idleClockWidth        = 42
	idleBatteryItemWidth  = 26
	idleBatteryStep       = idleBatteryItemWidth + idleCapsuleGap
)

type idleInfo struct {
	weather      idleWeather
	weatherUntil time.Time
	refreshAt    time.Time
	reading      bool
}

type idleWeather struct {
	condition   string
	temperature int
	icon        *ui.SVG
	rainDates   []string
}

func (i idleInfo) showsTomorrowRain(now time.Time) bool {
	return now.Before(i.weatherUntil) && len(i.weather.rainDates) > 0 &&
		slices.Contains(i.weather.rainDates, now.AddDate(0, 0, 1).Format(time.DateOnly))
}

func (a *island) showsIdleCapsule() bool {
	return !a.expanded && a.dock.side == dockNone && a.compactIdle()
}

func (a *island) idleCapsuleWidth() int {
	base, batteries := a.idleCapsuleMetrics(a.settings.idleItems(), a.idle.showsTomorrowRain(time.Now()))
	return max(compactHeight, base+batteries*idleBatteryStep)
}

func (a *island) idleCapsuleMetrics(items idleItemSettings, rainReminder bool) (baseWidth, batteries int) {
	baseWidth = 2*idleCapsulePadding - idleCapsuleGap
	for _, item := range items {
		if item.Hidden {
			continue
		}
		if item.ID == idleItemBattery {
			batteries = len(a.battery.readings)
		} else {
			baseWidth += idleItemSpecs[item.ID].width + idleCapsuleGap
			if item.ID == idleItemWeather && rainReminder {
				baseWidth += idleRainReminderWidth
			}
		}
	}
	return baseWidth, min(batteries, (panelWidth-baseWidth)/idleBatteryStep)
}

func (a *island) updateIdleWeather(c *ui.Context) {
	info := &a.idle
	if !a.showsIdleCapsule() || !a.settings.showsIdleItem(idleItemWeather) || !a.win.IsVisible() || info.reading {
		return
	}
	if delay := info.refreshAt.Sub(c.Now()); delay > 0 {
		c.After(delay)
		return
	}
	info.reading = true
	city := a.settings.WeatherCity
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		weather, err := fetchIdleWeather(ctx, city.weatherEndpoint())
		a.win.Update(func() {
			info.reading = false
			if city != a.settings.WeatherCity {
				return
			}
			now := time.Now()
			info.refreshAt = now.Add(5 * time.Minute)
			if err == nil {
				info.weather, info.weatherUntil = weather, now.Add(2*time.Hour)
				info.refreshAt = now.Add(30 * time.Minute)
			}
		})
	}()
}

func (a *island) idleCapsule(c *ui.Context, opacity float32) {
	items := a.settings.idleItems()
	now := c.Now()
	rainReminder := a.idle.showsTomorrowRain(now)
	base, batteries := a.idleCapsuleMetrics(items, rainReminder)
	if rainReminder && a.shownFrame.width < float32(base) {
		rainReminder = false
		base, batteries = a.idleCapsuleMetrics(items, false)
	}
	targetWidth := max(compactHeight, base+batteries*idleBatteryStep)
	minimumWidth := max(compactHeight, base)
	available := min(float32(targetWidth), max(float32(minimumWidth), a.shownFrame.width))
	batteries = min(batteries, int((available-float32(base))/idleBatteryStep))
	contentWidth := float32(base + batteries*idleBatteryStep)
	width := max(compactHeight, contentWidth)
	// Keep the trailing item aligned while more battery indicators come into view.
	left := max(0, (a.shownFrame.width-available)/2) + available - width
	x := left + (width-contentWidth)/2 + idleCapsulePadding
	ui.Row(c.Key("idle-status")).Absolute().Left(left).Top(0).Size(width, idleCapsuleHeight).
		Opacity(opacity).IgnoreInput(true).AlignItems(ui.Center).Justify(ui.Center).Gap(idleCapsuleGap).
		FontSize(13).FontWeight(600).FontFeatures("tnum").TextColor(ink).Padding(0, idleCapsulePadding).
		Children(func() {
			for _, item := range items {
				if item.Hidden {
					continue
				}
				itemWidth := idleItemSpecs[item.ID].width
				switch item.ID {
				case idleItemFace:
					slot := ui.Box(c.Key(item.ID)).Size(idleFaceWidth, idleCapsuleHeight).Shrink(0)
					a.drawIdleFace(c, slot, opacity, x+idleFaceWidth/2-a.shownFrame.width/2)
				case idleItemWeather:
					weather := a.idle.weather
					if rainReminder {
						itemWidth += idleRainReminderWidth
					}
					if now.Before(a.idle.weatherUntil) && len(weather.rainDates) > 0 {
						midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
						c.After(min(midnight.Sub(now), a.idle.weatherUntil.Sub(now)))
					}
					ui.Row(c.Key(item.ID)).Width(float32(itemWidth)).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Gap(3).Children(func() {
						if weather.condition != "" && now.Before(a.idle.weatherUntil) {
							ui.Icon(c.Key("condition"), weather.icon).Size(14, 14).Shrink(0).TextColor(ink).
								Role(ui.RoleImage).Label(weather.condition)
							ui.Text(c.Key("temperature"), fmt.Sprintf("%d°C", weather.temperature)).Shrink(0).SingleLine()
						}
						if rainReminder {
							ui.Text(c.Key("rain-reminder"), "明日有雨").Width(idleRainReminderWidth).Shrink(0).TextAlign(ui.Center).SingleLine()
						}
					})
				case idleItemBattery:
					if batteries > 0 {
						ui.Row(c.Key(item.ID)).AlignItems(ui.Center).Gap(idleCapsuleGap).Shrink(0).Children(func() {
							for _, reading := range a.battery.readings[:batteries] {
								batteryIndicator(c, reading, true)
							}
						})
					}
					x += float32(batteries * idleBatteryStep)
					continue
				case idleItemClock:
					c.After(now.Truncate(time.Minute).Add(time.Minute).Sub(now))
					ui.Text(c.Key(item.ID), now.Format("15:04")).Width(idleClockWidth).Shrink(0).TextAlign(ui.End).SingleLine()
				}
				x += float32(itemWidth + idleCapsuleGap)
			}
		})
}

func fetchIdleWeather(ctx context.Context, endpoint string) (idleWeather, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return idleWeather{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return idleWeather{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return idleWeather{}, fmt.Errorf("weather HTTP %d", resp.StatusCode)
	}
	type description struct{ Value string }
	var payload struct {
		Current []struct {
			Temperature string        `json:"temp_C"`
			Code        string        `json:"weatherCode"`
			Chinese     []description `json:"lang_zh-cn"`
			Localized   []description `json:"lang_xx"`
			Description []description `json:"weatherDesc"`
		} `json:"current_condition"`
		Forecast []struct {
			Date   string `json:"date"`
			Hourly []struct {
				Code string `json:"weatherCode"`
			} `json:"hourly"`
		} `json:"weather"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&payload); err != nil {
		return idleWeather{}, err
	}
	if len(payload.Current) == 0 {
		return idleWeather{}, errors.New("invalid weather response")
	}
	current := payload.Current[0]
	var condition string
	for _, descriptions := range [][]description{current.Chinese, current.Localized, current.Description} {
		if len(descriptions) > 0 {
			condition = strings.TrimSpace(descriptions[0].Value)
			if condition != "" {
				break
			}
		}
	}
	temperature, err := strconv.Atoi(strings.TrimSpace(current.Temperature))
	if condition == "" || len(condition) > 96 || strings.ContainsAny(condition, "\n\r\t") ||
		err != nil || temperature < -100 || temperature > 80 {
		return idleWeather{}, errors.New("invalid weather response")
	}
	code, _ := strconv.Atoi(strings.TrimSpace(current.Code))
	weather := idleWeather{condition: condition, temperature: temperature, icon: idleWeatherIcon(code, condition)}
	for _, day := range payload.Forecast {
		for _, hour := range day.Hourly {
			code, _ := strconv.Atoi(hour.Code)
			if idleWeatherIcon(code, "") == weatherRainIcon || code == 386 || code == 389 {
				weather.rainDates = append(weather.rainDates, day.Date)
				break
			}
		}
	}
	return weather, nil
}

func idleWeatherIcon(code int, condition string) *ui.SVG {
	switch code {
	case 113:
		return weatherSunIcon
	case 116, 119:
		return weatherPartlyCloudyIcon
	case 122:
		return weatherCloudIcon
	case 143, 248, 260:
		return weatherFogIcon
	case 176, 185, 263, 266, 281, 284, 293, 296, 299, 302, 305, 308, 311, 314, 353, 356, 359:
		return weatherRainIcon
	case 179, 182, 227, 230, 317, 320, 323, 326, 329, 332, 335, 338, 350, 362, 365, 368, 371, 374, 377, 395:
		return weatherSnowIcon
	case 200, 386, 389, 392:
		return weatherThunderIcon
	}
	// New conditions can appear before the service assigns them a documented code.
	name := strings.ToLower(condition)
	switch {
	case strings.Contains(name, "雷"), strings.Contains(name, "thunder"):
		return weatherThunderIcon
	case strings.ContainsAny(name, "雪霰雹"), strings.Contains(name, "snow"), strings.Contains(name, "sleet"), strings.Contains(name, "hail"):
		return weatherSnowIcon
	case strings.Contains(name, "雨"), strings.Contains(name, "rain"), strings.Contains(name, "drizzle"), strings.Contains(name, "shower"):
		return weatherRainIcon
	case strings.ContainsAny(name, "雾霧霾尘塵沙"), strings.Contains(name, "fog"), strings.Contains(name, "mist"), strings.Contains(name, "haze"), strings.Contains(name, "dust"), strings.Contains(name, "smoke"):
		return weatherFogIcon
	case strings.ContainsAny(name, "阴陰"), strings.Contains(name, "overcast"):
		return weatherCloudIcon
	case strings.ContainsAny(name, "云雲"), strings.Contains(name, "cloud"):
		return weatherPartlyCloudyIcon
	case strings.Contains(name, "晴"), name == "clear", name == "sunny":
		return weatherSunIcon
	default:
		return weatherUnknownIcon
	}
}
