package island

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	idleWeatherBase     = "https://wttr.in/"
	idleWeatherQuery    = "?format=%25C%7C%25t%7C%25i&lang=zh-cn&m"
	idleWeatherEndpoint = idleWeatherBase + idleWeatherQuery

	idleCapsulePadding   = 10
	idleCapsuleGap       = 2
	idleCapsuleHeight    = 40
	idleFaceWidth        = 30
	idleWeatherWidth     = 66
	idleClockWidth       = 42
	idleBatteryItemWidth = 26
	idleBatteryStep      = idleBatteryItemWidth + idleCapsuleGap
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
}

func (a *island) showsIdleCapsule() bool {
	return !a.expanded && a.dock.side == dockNone && a.compactIdle()
}

func (a *island) idleCapsuleWidth() int {
	base, batteries := a.idleCapsuleMetrics(a.settings.idleItems())
	return max(compactHeight, base+batteries*idleBatteryStep)
}

func (a *island) idleCapsuleMetrics(items idleItemSettings) (baseWidth, batteries int) {
	baseWidth = 2*idleCapsulePadding - idleCapsuleGap
	for _, item := range items {
		if item.Hidden {
			continue
		}
		if item.ID == idleItemBattery {
			batteries = len(a.battery.readings)
		} else {
			baseWidth += idleItemSpecs[item.ID].width + idleCapsuleGap
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
	base, batteries := a.idleCapsuleMetrics(items)
	targetWidth := max(compactHeight, base+batteries*idleBatteryStep)
	minimumWidth := max(compactHeight, base)
	available := min(float32(targetWidth), max(float32(minimumWidth), a.shownFrame.width))
	batteries = min(batteries, int((available-float32(base))/idleBatteryStep))
	contentWidth := float32(base + batteries*idleBatteryStep)
	width := max(compactHeight, contentWidth)
	// Keep the trailing item aligned while more battery indicators come into view.
	left := max(0, (a.shownFrame.width-available)/2) + available - width
	x := left + (width-contentWidth)/2 + idleCapsulePadding
	now := c.Now()
	ui.Row(c.Key("idle-status")).Absolute().Left(left).Top(0).Size(width, idleCapsuleHeight).
		Opacity(opacity).IgnoreInput(true).AlignItems(ui.Center).Justify(ui.Center).Gap(idleCapsuleGap).
		FontSize(13).FontWeight(600).FontFeatures("tnum").TextColor(ink).Padding(0, idleCapsulePadding).
		Children(func() {
			for _, item := range items {
				if item.Hidden {
					continue
				}
				switch item.ID {
				case idleItemFace:
					slot := ui.Box(c.Key(item.ID)).Size(idleFaceWidth, idleCapsuleHeight).Shrink(0)
					a.drawIdleFace(c, slot, opacity, x+idleFaceWidth/2-a.shownFrame.width/2)
				case idleItemWeather:
					ui.Row(c.Key(item.ID)).Width(idleWeatherWidth).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Gap(3).Children(func() {
						if weather := a.idle.weather; weather.condition != "" && now.Before(a.idle.weatherUntil) {
							ui.Icon(c.Key("condition"), weather.icon).Size(14, 14).Shrink(0).TextColor(ink).
								Role(ui.RoleImage).Label(weather.condition)
							ui.Text(c.Key("temperature"), fmt.Sprintf("%d°C", weather.temperature)).Shrink(0).SingleLine()
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
				x += float32(idleItemSpecs[item.ID].width + idleCapsuleGap)
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return idleWeather{}, err
	}
	condition, rest, found := strings.Cut(strings.TrimSpace(string(data)), "|")
	reading, rawCode, _ := strings.Cut(rest, "|")
	condition = strings.TrimSpace(condition)
	number, celsius := strings.CutSuffix(strings.TrimSpace(reading), "°C")
	temperature, err := strconv.Atoi(number)
	if !found || condition == "" || len(condition) > 96 || strings.ContainsAny(condition, "\n\r\t") ||
		!celsius || err != nil || temperature < -100 || temperature > 80 {
		return idleWeather{}, errors.New("invalid weather response")
	}
	code, _ := strconv.Atoi(strings.TrimSpace(rawCode))
	return idleWeather{condition: condition, temperature: temperature, icon: idleWeatherIcon(code, condition)}, nil
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
