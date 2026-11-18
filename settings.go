package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	waveDefault = iota
	waveCustom
	waveArtwork
)

const (
	mediaCardDefault = iota
	mediaCardLargeCover
)

const (
	settingsWidth        = 432
	settingsControlWidth = 236
)

type appSettings struct {
	Mode           int         `json:"waveColorMode"`
	Color          ui.Color    `json:"waveColor"`
	HideLyrics     bool        `json:"hideLyrics,omitempty"`
	WeatherCity    weatherCity `json:"weatherCity,omitzero"`
	MediaCardStyle int         `json:"mediaCardStyle,omitempty"`
}

func (s appSettings) waveColor(artwork ui.Color) ui.Color {
	switch s.Mode {
	case waveCustom:
		return s.Color
	case waveArtwork:
		if artwork.A != 0 {
			lightness := 0.2126*float32(artwork.R) + 0.7152*float32(artwork.G) + 0.0722*float32(artwork.B)
			if lightness > 125 {
				return artwork.Mix(ui.RGB(0, 0, 0), 1-125/lightness)
			}
			return artwork
		}
	}
	return accent
}

func loadSettings(path string) (appSettings, error) {
	defaults := appSettings{Color: accent}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	settings := defaults
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaults, err
	}
	if settings.Mode < waveDefault || settings.Mode > waveArtwork {
		return defaults, fmt.Errorf("未知的律动配色模式 %d", settings.Mode)
	}
	if !settings.WeatherCity.valid() {
		settings.WeatherCity = weatherCity{}
	}
	if settings.MediaCardStyle != mediaCardLargeCover {
		settings.MediaCardStyle = mediaCardDefault
	}
	return settings, nil
}

func (a *island) settingsPath() string {
	return filepath.Join(filepath.Dir(a.statePath), "settings.json")
}

func (a *island) saveSettings() {
	if a.settingsSaveAt.IsZero() {
		return
	}
	a.settingsSaveAt = time.Time{}
	err := a.settings.save(a.settingsPath())
	a.settingsSaveFailed = err != nil
	if err != nil {
		log.Printf("save settings: %v", err)
	}
}

func (s appSettings) save(path string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp := path + ".tmp"
	defer os.Remove(temp)
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func (a *island) settingsHeight() int {
	if a.settings.Mode == waveCustom {
		return 620
	}
	return 286
}

func (a *island) openSettings() {
	if a.settingsWindow == nil {
		area := mygo.Screen.PrimaryDisplay().WorkArea
		a.settingsWindow = mygo.NewWindow(mygo.WindowOptions{
			Title: "灵动岛设置", Content: ui.View(a.settingsView),
			Width: settingsWidth, Height: min(a.settingsHeight(), area.Height-48), UseContentSize: true,
			DisableResize: true, DisableMaximize: true, DisableFullScreen: true,
			BackgroundColor: "#f2f3f5",
		})
		a.settingsWindow.OnClose(func(*mygo.CloseEvent) { a.saveSettings() })
		a.settingsWindow.OnClosed(func() {
			a.settingsWindow = nil
			a.citySearch = weatherCitySearch{}
		})
	} else {
		a.settingsWindow.Show()
	}
	a.settingsWindow.Focus()
}

func settingsGroup(c *ui.Context, label string, children func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Text(c, label).Height(16).Padding(0, 3).FontSize(11.5).FontWeight(600).TextColor(muted)
		ui.Column(c).FillWidth().Radius(12).Background(white.Alpha(0.72)).
			Border(1, theme.Border).Children(children)
	})
}

func settingsRow(c *ui.Context) ui.Element {
	return ui.Row(c).FillWidth().Height(46).Shrink(0).Padding(0, 14).Gap(12).AlignItems(ui.Center)
}

func (a *island) settingsView(c *ui.Context) {
	c.SetTheme(theme)
	c.Root().Background(ui.Hex("#f2f3f5"))
	ui.Scroll(c.Key("settings")).Fill().Padding(16, 18).Gap(14).Children(func() {
		var weatherChanged, lyricsChanged, cardStyleChanged, modeChanged, colorChanged bool
		settingsGroup(c.Key("idle-settings"), "空闲", func() { weatherChanged = a.weatherSettings(c) })
		settingsGroup(c.Key("media-settings"), "媒体", func() {
			divider := func() {
				ui.Box(c).AlignSelf(ui.Stretch).Height(1).Shrink(0).Margin(0, 14).Background(theme.Border)
			}
			settingsRow(c.Key("card-style")).Children(func() {
				ui.Text(c, "音乐卡片风格").Grow(1).FontSize(13).TextColor(ink)
				cardStyleChanged = ui.Segmented(c.Key("card-mode"), &a.settings.MediaCardStyle, "默认风格", "大封面").
					FillEqually().Width(settingsControlWidth).Height(30).FontSize(12).Label("音乐卡片选项").Changed()
			})
			divider()
			settingsRow(c.Key("lyric-settings")).Children(func() {
				ui.Text(c, "显示歌词").Grow(1).FontSize(13).TextColor(ink)
				shown := !a.settings.HideLyrics
				if ui.Switch(c.Key("show-lyrics"), &shown).Label("歌词显示").Changed() {
					a.settings.HideLyrics = !shown
					lyricsChanged = true
				}
			})
			divider()
			settingsRow(c.Key("wave-settings")).Children(func() {
				ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "律动柱颜色").FontSize(13).SingleLine().Shrink(0).TextColor(ink)
					ui.Box(c.Key("wave-preview")).Size(38, 22).Shrink(0).Label("律动预览").
						Draw(func(p *ui.Painter, r ui.Rect) {
							levels := [...]float32{0.16, 0.38, 0.62, 0.52, 0.68, 0.40, 0.14}
							paintMediaBars(p, r, levels[:], 1, 1, a.settings.waveColor(a.media.artworkColor))
						})
				})
				modeChanged = ui.Segmented(c.Key("wave-mode"), &a.settings.Mode, "默认", "自定义", "跟随海报").
					FillEqually().Width(settingsControlWidth).Height(30).FontSize(12).Label("律动配色模式").Changed()
			})
			if a.settings.Mode == waveCustom {
				divider()
				ui.Column(c).FillWidth().Padding(10, 14, 14).Children(func() {
					colorChanged = ui.ColorPicker(c.Key("wave-color"), &a.settings.Color).
						FillWidth().Label("自定义律动柱颜色").Changed()
				})
			}
		})
		if modeChanged && a.settingsWindow != nil {
			win := a.settingsWindow
			win.Update(func() {
				frame, content := win.Bounds(), win.ContentBounds()
				area := mygo.Screen.DisplayNearestPoint(mygo.Point{X: frame.X + frame.Width/2, Y: frame.Y + frame.Height/2}).WorkArea
				chrome := frame.Height - content.Height
				height := min(a.settingsHeight(), area.Height-chrome)
				top := min(max(frame.Y, area.Y), area.Y+area.Height-height-chrome)
				content.Y += top - frame.Y
				content.Height = height
				win.SetContentBounds(content)
			})
		}
		if modeChanged || colorChanged || lyricsChanged || weatherChanged || cardStyleChanged {
			a.settingsSaveAt = c.Now().Add(300 * time.Millisecond)
			if a.win != nil {
				a.win.Invalidate()
			}
		}
		if !a.settingsSaveAt.IsZero() {
			if remaining := a.settingsSaveAt.Sub(c.Now()); remaining > 0 {
				c.After(remaining)
			} else {
				a.saveSettings()
			}
		}
		if a.settingsSaveFailed {
			ui.Text(c, "设置保存失败").FontSize(11).TextColor(ui.Hex("#a63b43"))
		}
	})
}
