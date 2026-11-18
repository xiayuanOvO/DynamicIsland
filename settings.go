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

const settingsWidth = 328

type appSettings struct {
	Mode  int      `json:"waveColorMode"`
	Color ui.Color `json:"waveColor"`
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
		return 496
	}
	return 176
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
		a.settingsWindow.OnClosed(func() { a.settingsWindow = nil })
	} else {
		a.settingsWindow.Show()
	}
	a.settingsWindow.Focus()
}

func (a *island) settingsView(c *ui.Context) {
	c.SetTheme(theme)
	c.Root().Background(ui.Hex("#f2f3f5"))
	ui.Scroll(c.Key("settings")).Fill().Padding(16, 20).Gap(10).Children(func() {
		ui.Text(c, "律动柱颜色").FontSize(15).FontWeight(600).TextColor(ink)
		modeChanged := ui.Segmented(c, &a.settings.Mode, "默认", "自定义", "跟随海报").
			FillEqually().Label("律动配色模式").Changed()
		preview := ui.Box(c.Key("wave-preview")).FillWidth().Height(compactHeight).
			Shrink(0).Radius(windowRadius).Clip().Center()
		glassSurface(preview, false, false)
		preview.Children(func() {
			ui.Box(c).Size(78, 24).Draw(func(p *ui.Painter, r ui.Rect) {
				levels := [...]float32{0.1, 0.26, 0.44, 0.62, 0.36, 0.48, 0.70, 0.52, 0.31, 0.42, 0.25, 0.33, 0.12}
				paintMediaBars(p, r, levels[:], 1, 1, a.settings.waveColor(a.media.artworkColor))
			})
		})
		colorChanged := false
		if a.settings.Mode == waveCustom {
			colorChanged = ui.ColorPicker(c.Key("wave-color"), &a.settings.Color).
				FillWidth().Label("自定义律动柱颜色").Changed()
		}
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
		if modeChanged || colorChanged {
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
