package island

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
	settingsWidth  = 584
	settingsHeight = 520
)

type appSettings struct {
	Theme            int              `json:"theme,omitempty"`
	Mode             int              `json:"waveColorMode"`
	Color            ui.Color         `json:"waveColor"`
	HideLyrics       bool             `json:"hideLyrics,omitempty"`
	LyricTransition  int              `json:"lyricTransition,omitempty"`
	LyricStyle       lyricStyle       `json:"lyricStyle,omitzero"`
	WeatherCity      weatherCity      `json:"weatherCity,omitzero"`
	IdleItems        idleItemSettings `json:"idleItems,omitzero"`
	MediaCardStyle   int              `json:"mediaCardStyle,omitempty"`
	HideInFullscreen bool             `json:"hideInFullscreen"`
}

func (s appSettings) waveColor(artwork ui.Color) ui.Color {
	switch s.Mode {
	case waveCustom:
		return s.Color
	case waveArtwork:
		if artwork.A != 0 {
			lightness := 0.2126*float32(artwork.R) + 0.7152*float32(artwork.G) + 0.0722*float32(artwork.B)
			if s.Theme == themeDark {
				if lightness < 155 {
					return artwork.Mix(white, (155-lightness)/(255-lightness))
				}
			} else if lightness > 125 {
				return artwork.Mix(ui.RGB(0, 0, 0), 1-125/lightness)
			}
			return artwork
		}
	}
	return islandThemes[s.Theme].Accent
}

func loadSettings(path string) (appSettings, error) {
	defaults := appSettings{Color: accent, HideInFullscreen: true}
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
	if settings.Theme != themeDark {
		settings.Theme = themeLight
	}
	if !settings.WeatherCity.valid() {
		settings.WeatherCity = weatherCity{}
	}
	settings.IdleItems = settings.IdleItems.normalized()
	if settings.MediaCardStyle != mediaCardLargeCover {
		settings.MediaCardStyle = mediaCardDefault
	}
	if settings.LyricTransition < lyricTransitionSlide || settings.LyricTransition > lyricTransitionNone {
		settings.LyricTransition = lyricTransitionSlide
	}
	if settings.LyricStyle.FontSize < lyricMinFontSize || settings.LyricStyle.FontSize > lyricMaxFontSize {
		settings.LyricStyle.FontSize = 0
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
	return saveJSON(path, s)
}

func saveJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return saveStateFile(path, data)
}

func saveStateFile(path string, data []byte) error {
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

func (a *island) openSettings() {
	a.refreshLyricCacheSize()
	if a.settingsWindow == nil {
		area := mygo.Screen.PrimaryDisplay().WorkArea
		a.settingsWindow = mygo.NewWindow(mygo.WindowOptions{
			Title: "灵动岛设置", Content: ui.View(a.settingsView),
			Width: settingsWidth, Height: min(settingsHeight, area.Height-48), UseContentSize: true,
			DisableResize: true, DisableMaximize: true, DisableFullScreen: true,
			BackgroundColor: "light-dark(" + settingsBackgroundLight + ", " + settingsBackgroundDark + ")",
		})
		a.settingsWindow.OnClose(func(*mygo.CloseEvent) { a.saveSettings() })
		a.settingsWindow.OnClosed(func() {
			a.settingsWindow = nil
			a.citySearch = weatherCitySearch{}
			a.idleSettingsList = ui.ListState{}
			a.clipboard.editingShortcut, a.clipboard.shortcutProblem = false, ""
			if a.updates.phase == updateLatest {
				a.updates.phase = updateIdle
			}
		})
	} else {
		a.settingsWindow.Show()
	}
	a.settingsWindow.Focus()
}

func (a *island) refreshLyricCacheSize() {
	a.lyrics.cacheBytes, a.lyrics.cacheSizeErr = a.lyrics.diskCache.Size()
	if a.settingsWindow != nil {
		a.settingsWindow.Invalidate()
	}
}
