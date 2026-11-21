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
	idleWeatherQuery    = "?format=%25C%7C%25t&lang=zh-cn&m"
	idleWeatherEndpoint = idleWeatherBase + idleWeatherQuery

	idleCapsulePadding   = 12
	idleCapsuleGap       = 4
	idleFaceWidth        = 36
	idleWeatherWidth     = 76
	idleBatteryItemWidth = 32
	idleCapsuleBaseWidth = 2*idleCapsulePadding + idleFaceWidth + idleCapsuleGap + idleWeatherWidth
	idleMaxBatteries     = (panelWidth - idleCapsuleBaseWidth) / (idleBatteryItemWidth + idleCapsuleGap)
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
}

func (a *island) showsIdleCapsule() bool {
	return !a.expanded && a.dock.side == dockNone && a.compactIdle()
}

func (a *island) idleCapsuleWidth() int {
	return idleCapsuleBaseWidth + min(idleMaxBatteries, len(a.battery.readings))*(idleBatteryItemWidth+idleCapsuleGap)
}

func (a *island) updateIdleWeather(c *ui.Context) {
	info := &a.idle
	if !a.showsIdleCapsule() || !a.win.IsVisible() || info.reading {
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
	width := float32(a.idleCapsuleWidth())
	left := max(0, (a.shownFrame.width-width)/2)
	eyeOffset := left + idleCapsulePadding + idleFaceWidth/2 - a.shownFrame.width/2
	a.drawIdleFace(c, compactHeight, 1-opacity, eyeOffset)
	now := c.Now()
	c.After(now.Truncate(time.Minute).Add(time.Minute).Sub(now))
	ui.Row(c.Key("idle-status")).Absolute().Left(left).Top(0).Size(width, compactHeight).
		Opacity(opacity).IgnoreInput(true).AlignItems(ui.Center).Gap(idleCapsuleGap).
		Padding(0, idleCapsulePadding, 0, idleCapsulePadding+idleFaceWidth+idleCapsuleGap).Children(func() {
		ui.Column(c.Key("idle-clock-and-weather")).Width(idleWeatherWidth).Shrink(0).Children(func() {
			ui.Text(c.Key("clock"), now.Format("15:04")).FontSize(16).FontWeight(650).
				FontFeatures("tnum").SingleLine().TextColor(ink)
			if weather := a.idle.weather; weather.condition != "" && now.Before(a.idle.weatherUntil) {
				ui.Row(c.Key("weather")).FillWidth().Gap(4).Children(func() {
					ui.Text(c.Key("condition"), weather.condition).FontSize(11).FontWeight(500).
						MinWidth(0).Shrink(1).SingleLine().Ellipsis("…").TextColor(ink)
					ui.Text(c.Key("temperature"), fmt.Sprintf("%d°C", weather.temperature)).FontSize(11).FontWeight(500).
						Shrink(0).SingleLine().TextColor(ink)
				})
			}
		})
		for _, reading := range a.battery.readings[:min(idleMaxBatteries, len(a.battery.readings))] {
			batteryIndicator(c, reading, true)
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
	condition, reading, found := strings.Cut(strings.TrimSpace(string(data)), "|")
	condition = strings.TrimSpace(condition)
	number, celsius := strings.CutSuffix(strings.TrimSpace(reading), "°C")
	temperature, err := strconv.Atoi(number)
	if !found || condition == "" || len(condition) > 96 || strings.ContainsAny(condition, "\n\r\t") ||
		!celsius || err != nil || temperature < -100 || temperature > 80 {
		return idleWeather{}, errors.New("invalid weather response")
	}
	return idleWeather{condition: condition, temperature: temperature}, nil
}
