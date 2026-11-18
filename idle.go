package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	idleInfoWidth       = 148
	idleWeatherBase     = "https://wttr.in/"
	idleWeatherQuery    = "?format=%25C%7C%25t&lang=zh-cn&m"
	idleWeatherEndpoint = idleWeatherBase + idleWeatherQuery
)

type idleInfo struct {
	win          *mygo.Window
	anchor       mygo.Rectangle
	align        ui.Align
	weather      string
	weatherUntil time.Time
	refreshAt    time.Time
	reading      bool
}

func (a *island) showsIdleInfo() bool {
	return !a.expanded && a.compactIdle()
}

func (a *island) updateIdleInfo(c *ui.Context, frame islandFrame, width, height int) {
	info := &a.idle
	var anchor mygo.Rectangle
	if a.showsIdleInfo() && frame.open == 0 && a.win.IsVisible() {
		anchor = a.windowBounds(width, height)
		now := c.Now()
		if !info.reading {
			if delay := info.refreshAt.Sub(now); delay > 0 {
				c.After(delay)
			} else {
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
						if info.win != nil {
							info.win.Invalidate()
						}
					})
				}()
			}
		}
	}
	if anchor == info.anchor {
		return
	}
	info.anchor = anchor
	a.win.Update(func() {
		var bounds mygo.Rectangle
		if info.anchor.Width > 0 && a.win.IsVisible() && a.showsIdleInfo() {
			display := a.dock.display
			if a.dock.side == dockNone {
				display = mygo.Screen.DisplayMatching(info.anchor)
			}
			bounds, info.align = idleInfoBounds(info.anchor, display.WorkArea)
		}
		a.placeOverlay(&info.win, bounds, "时间与天气", info.view)
	})
}

func idleInfoBounds(ball, area mygo.Rectangle) (mygo.Rectangle, ui.Align) {
	const gap = 10
	align := ui.Start
	rightStart := max(ball.X+ball.Width+gap, area.X)
	leftEnd := min(ball.X-gap, area.X+area.Width)
	rightSpace, leftSpace := area.X+area.Width-rightStart, leftEnd-area.X
	x, width := rightStart, min(idleInfoWidth, rightSpace)
	if rightSpace < 96 && leftSpace > rightSpace {
		width = min(idleInfoWidth, leftSpace)
		x, align = leftEnd-width, ui.End
	}
	height := min(compactHeight, area.Height)
	y := min(max(ball.Y+(ball.Height-height)/2, area.Y), area.Y+area.Height-height)
	return mygo.Rectangle{X: x, Y: y, Width: width, Height: height}, align
}

func (i *idleInfo) view(c *ui.Context) {
	c.SetTheme(theme)
	c.Root().Background(ui.Transparent)
	now := c.Now()
	c.After(now.Truncate(time.Minute).Add(time.Minute).Sub(now))
	ui.Column(c.Key("idle-info")).Fill().Padding(2, 4).Justify(ui.Center).Children(func() {
		ui.Text(c.Key("clock"), now.Format("15:04")).FillWidth().FontSize(22).FontWeight(650).
			FontFeatures("tnum").SingleLine().TextAlign(i.align).TextColor(white).
			TextShadow(0, 1, 6, ink.Alpha(0.95))
		if i.weather != "" && now.Before(i.weatherUntil) {
			ui.Text(c.Key("weather"), i.weather).FillWidth().FontSize(11.5).FontWeight(500).
				SingleLine().Ellipsis("…").TextAlign(i.align).TextColor(white.Alpha(0.86)).
				TextShadow(0, 1, 5, ink.Alpha(0.92))
		}
	})
}

func fetchIdleWeather(ctx context.Context, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("weather HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	condition, reading, found := strings.Cut(strings.TrimSpace(string(data)), "|")
	condition = strings.TrimSpace(condition)
	number, celsius := strings.CutSuffix(strings.TrimSpace(reading), "°C")
	temperature, err := strconv.Atoi(number)
	if !found || condition == "" || len(condition) > 96 || strings.ContainsAny(condition, "\n\r\t") ||
		!celsius || err != nil || temperature < -100 || temperature > 80 {
		return "", errors.New("invalid weather response")
	}
	return fmt.Sprintf("%s %d°C", condition, temperature), nil
}
