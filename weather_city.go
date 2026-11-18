package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const weatherCitySearchEndpoint = "https://geocoding-api.open-meteo.com/v1/search"
const weatherCityAttribution = "https://open-meteo.com/en/docs/geocoding-api#attribution"

type weatherCity struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Region    string  `json:"region"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (c weatherCity) valid() bool {
	return c.ID > 0 && strings.TrimSpace(c.Name) != "" &&
		c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}

func (c weatherCity) label() string {
	if c.Region != "" {
		return c.Name + " · " + c.Region
	}
	return c.Name
}

func (c weatherCity) weatherEndpoint() string {
	if c.ID == 0 {
		return idleWeatherEndpoint
	}
	return fmt.Sprintf("%s~%.5f,%.5f%s", idleWeatherBase, c.Latitude, c.Longitude, idleWeatherQuery)
}

func searchWeatherCities(ctx context.Context, endpoint, name string) ([]weatherCity, error) {
	params := url.Values{"name": {name}, "count": {"5"}, "language": {"zh"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("city search HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Results []struct {
			ID                                    int
			Name, Admin1, Admin2, Admin3, Country string
			Latitude, Longitude                   *float64
		}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&payload); err != nil {
		return nil, err
	}
	cities := make([]weatherCity, 0, len(payload.Results))
	for _, result := range payload.Results {
		if result.Latitude == nil || result.Longitude == nil {
			continue
		}
		city := weatherCity{
			ID: result.ID, Name: strings.TrimSpace(result.Name),
			Latitude: *result.Latitude, Longitude: *result.Longitude,
		}
		if !city.valid() {
			continue
		}
		regions := make([]string, 0, 4)
		for _, region := range []string{result.Admin3, result.Admin2, result.Admin1, result.Country} {
			if region != "" && region != city.Name && !slices.Contains(regions, region) {
				regions = append(regions, region)
			}
		}
		city.Region = strings.Join(regions, " · ")
		cities = append(cities, city)
	}
	return cities, nil
}

type weatherCitySearch struct {
	open     bool
	query    string
	searched string
	results  []weatherCity
	loading  bool
	failed   bool
}

func (a *island) weatherSettings(c *ui.Context) bool {
	search := &a.citySearch
	selected := a.settings.WeatherCity
	settingsRow(c.Key("weather-city")).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "天气城市").FontSize(13).TextColor(ink)
			if selected.ID != 0 {
				ui.Link(c, "Open-Meteo", weatherCityAttribution).FontSize(10).TextColor(muted).SingleLine()
			}
		})
		label := "自动定位"
		if selected.ID != 0 {
			label = selected.Name
		}
		trigger := ui.Button(c.Key("city-picker"), label).Width(settingsControlWidth).Height(30).FontSize(12).Clip()
		if selected.ID != 0 {
			trigger.Tooltip(selected.label())
		}
		if trigger.Clicked() {
			search.open = !search.open
		}
		ui.PopoverBase(c, trigger, &search.open, func(panel ui.Element) {
			panel.AttachTo(trigger, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0).
				Width(268).Radius(12).Background(ui.Hex("#f2f3f5")).Border(1, theme.Border).
				Shadow(0, 4, 14, 0, ink.Alpha(0.16)).Clip()
			ui.Column(c).FillWidth().Padding(8).Gap(4).Children(func() {
				ui.Row(c).FillWidth().Height(30).Gap(4).AlignItems(ui.Center).Children(func() {
					field := ui.SearchField(c.Key("city-search"), &search.query).Grow(1).MinWidth(0).
						Height(30).Label("搜索城市").Placeholder("搜索城市")
					query := strings.TrimSpace(search.query)
					submitted := field.Submitted()
					button := ui.Button(c.Key("search-city"), "搜索").Height(30).Disabled(query == "" || search.loading)
					if (button.Clicked() || submitted) && query != "" && !search.loading && a.settingsWindow != nil {
						win := a.settingsWindow
						search.searched, search.loading, search.failed = query, true, false
						search.results = nil
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
							defer cancel()
							cities, err := searchWeatherCities(ctx, weatherCitySearchEndpoint, query)
							win.Update(func() {
								if a.settingsWindow == win {
									search.results, search.loading, search.failed = cities, false, err != nil
								}
							})
						}()
					}
				})
				if selected.ID != 0 && ui.Button(c.Key("auto-weather-city"), "恢复自动定位").Height(26).FillWidth().Clicked() {
					selected = weatherCity{}
					search.open = false
				}
				if search.searched == "" || search.searched != strings.TrimSpace(search.query) {
					return
				}
				status := ""
				switch {
				case search.loading:
					status = "搜索中…"
				case search.failed:
					status = "城市搜索失败"
				case len(search.results) == 0:
					status = "未找到城市"
				}
				if status != "" {
					ui.Text(c, status).Padding(4, 6).FontSize(11).TextColor(muted)
					return
				}
				ui.Scroll(c.Key("city-results")).FillWidth().Height(float32(min(3, len(search.results)) * 37)).Gap(2).Children(func() {
					for _, city := range search.results {
						result := ui.ButtonBase(c.Key(city.ID)).Column().FillWidth().Height(35).Shrink(0).
							Padding(2, 8).Justify(ui.Center).Radius(7).Label(city.label()).Cursor(ui.CursorPointer)
						if result.Hovered() || city.ID == selected.ID {
							result.Background(white.Alpha(0.76))
						}
						result.Children(func() {
							ui.Text(c, city.Name).FontSize(12).FontWeight(600).SingleLine().TextColor(ink)
							if city.Region != "" {
								ui.Text(c, city.Region).FontSize(10).SingleLine().Ellipsis("…").TextColor(muted)
							}
						})
						if result.Clicked() {
							selected = city
							search.open = false
						}
					}
				})
				ui.Link(c, "Open-Meteo · GeoNames", weatherCityAttribution).FontSize(10).TextColor(muted).SingleLine()
			})
		})
	})
	if selected == a.settings.WeatherCity {
		return false
	}
	a.settings.WeatherCity = selected
	a.idle.weather, a.idle.weatherUntil, a.idle.refreshAt = "", time.Time{}, time.Time{}
	if a.idle.win != nil {
		a.idle.win.Invalidate()
	}
	return true
}
