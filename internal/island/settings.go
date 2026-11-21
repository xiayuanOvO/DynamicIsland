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
	settingsWidth           = 432
	settingsControlWidth    = 236
	settingsRowHeight       = 46
	clipboardSettingsHeight = 128
	cardPreviewInset        = 8
	cardPreviewGap          = 12
	settingsBackground      = "#f2f3f5"
)

var settingsTheme = func() *ui.Theme {
	t := *theme
	t.Background = ui.Hex(settingsBackground)
	return &t
}()

type appSettings struct {
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
			if lightness > 125 {
				return artwork.Mix(ui.RGB(0, 0, 0), 1-125/lightness)
			}
			return artwork
		}
	}
	return accent
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

func (a *island) settingsHeight() int {
	height := 415 + a.settings.cardHeight() + 2*cardPreviewInset + cardPreviewGap + clipboardSettingsHeight
	height += len(defaultIdleItems)*settingsRowHeight + 1
	if supportsFullscreenHiding {
		height += settingsRowHeight + 1
	}
	if !a.settings.HideLyrics {
		height += 3 * (settingsRowHeight + 1)
	}
	if a.settings.Mode == waveCustom {
		height += 334
	}
	return height
}

func (a *island) openSettings() {
	a.refreshLyricCacheSize()
	if a.settingsWindow == nil {
		area := mygo.Screen.PrimaryDisplay().WorkArea
		a.settingsWindow = mygo.NewWindow(mygo.WindowOptions{
			Title: "灵动岛设置", Content: ui.View(a.settingsView),
			Width: settingsWidth, Height: min(a.settingsHeight(), area.Height-48), UseContentSize: true,
			DisableResize: true, DisableMaximize: true, DisableFullScreen: true,
			BackgroundColor: settingsBackground,
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

func settingsGroup(c *ui.Context, label string, children func()) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Text(c, label).Height(16).Padding(0, 3).FontSize(11.5).FontWeight(600).TextColor(muted)
		ui.Column(c).FillWidth().Radius(12).Background(white.Alpha(0.72)).
			Border(1, theme.Border).Children(children)
	})
}

func settingsRow(c *ui.Context) ui.Element {
	return ui.Row(c).FillWidth().Height(settingsRowHeight).Shrink(0).Padding(0, 14).Gap(12).AlignItems(ui.Center)
}

func (a *island) settingsView(c *ui.Context) {
	c.SetTheme(settingsTheme)
	c.Root().Background(settingsTheme.Background)
	ui.Scroll(c.Key("settings")).Fill().Padding(16, 18).Gap(14).Children(func() {
		var idleChanged, weatherChanged, lyricsChanged, transitionChanged, cardStyleChanged, modeChanged, colorChanged bool
		var lyricSizeChanged, lyricColorChanged, fullscreenChanged bool
		settingsGroup(c.Key("idle-settings"), "空闲", func() {
			idleChanged = a.idleDisplaySettings(c)
			ui.Box(c).AlignSelf(ui.Stretch).Height(1).Shrink(0).Margin(0, 14).Background(theme.Border)
			weatherChanged = a.weatherSettings(c)
		})
		settingsGroup(c.Key("clipboard-settings"), "剪贴板", func() { a.clipboardSettings(c) })
		settingsGroup(c.Key("media-settings"), "媒体", func() {
			divider := func() {
				ui.Box(c).AlignSelf(ui.Stretch).Height(1).Shrink(0).Margin(0, 14).Background(theme.Border)
			}
			settingsRow(c.Key("card-style")).Children(func() {
				ui.Text(c, "音乐卡片风格").Grow(1).FontSize(13).TextColor(ink)
				cardStyleChanged = ui.Segmented(c.Key("card-mode"), &a.settings.MediaCardStyle, "默认风格", "大封面").
					FillEqually().Width(settingsControlWidth).Height(30).FontSize(12).Label("音乐卡片选项").Changed()
			})
			ui.Column(c.Key("card-preview-area")).FillWidth().Padding(0, 14, cardPreviewGap).Children(func() {
				ui.Column(c.Key("card-preview")).FillWidth().Padding(cardPreviewInset).
					Radius(12).Background(ui.Hex("#e5e8ec")).Label("音乐卡片预览").
					Children(func() { a.mediaCardPreview(c) })
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
			if !a.settings.HideLyrics {
				style := &a.settings.LyricStyle
				divider()
				settingsRow(c.Key("lyric-size-settings")).Children(func() {
					ui.Text(c, "歌词字号").Grow(1).FontSize(13).TextColor(ink).AccessHidden(true)
					ui.Row(c).Width(settingsControlWidth).Gap(8).AlignItems(ui.Center).Children(func() {
						size := float64(style.fontSize())
						lyricSizeChanged = ui.Slider(c.Key("lyric-font-size"), &size, lyricMinFontSize, lyricMaxFontSize).
							Step(1).Grow(1).Height(24).Label("歌词字号").Changed()
						if lyricSizeChanged {
							style.FontSize = int(size)
						}
						ui.Text(c, fmt.Sprint(style.fontSize())).Width(20).FontSize(12).TextAlign(ui.End).TextColor(muted).AccessHidden(true)
					})
				})
				divider()
				settingsRow(c.Key("lyric-color-settings")).Children(func() {
					ui.Text(c, "歌词高亮颜色").Grow(1).FontSize(13).TextColor(ink).AccessHidden(true)
					color := style.highlightColor()
					if color != white && ui.Button(c.Key("reset-lyric-color"), "恢复默认").Height(36).FontSize(12).
						Label("恢复默认歌词颜色").Clicked() {
						style.Color, color = nil, white
						lyricColorChanged = true
					}
					if ui.ColorWell(c.Key("lyric-highlight-color"), &color).Height(36).Label("歌词高亮颜色").Changed() {
						style.Color = &color
						lyricColorChanged = true
					}
				})
				divider()
				settingsRow(c.Key("lyric-transition-settings")).Children(func() {
					ui.Text(c, "歌词过渡").Grow(1).FontSize(13).TextColor(ink)
					transitionChanged = ui.Segmented(c.Key("lyric-transition"), &a.settings.LyricTransition, "上移", "淡入", "缩放", "关闭").
						FillEqually().Width(settingsControlWidth).Height(36).FontSize(12).Label("歌词过渡").Changed()
				})
			}
			divider()
			settingsRow(c.Key("lyric-cache")).Children(func() {
				lyrics := &a.lyrics
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, "歌词缓存").FontSize(13).TextColor(ink)
					if lyrics.cacheClearErr != nil {
						ui.Text(c, "清理失败，可重试").FontSize(11).TextColor(ui.Hex("#a63b43"))
					}
				})
				const bytesPerMB = 1_000_000
				size := fmt.Sprintf("%.2f MB", float64(lyrics.cacheBytes)/bytesPerMB)
				if lyrics.cacheSizeErr != nil {
					size = "无法读取大小"
				} else if lyrics.cacheBytes > 0 && lyrics.cacheBytes < bytesPerMB/100 {
					size = "< 0.01 MB"
				}
				ui.Text(c, size).FontSize(12).TextColor(muted)
				if ui.Button(c.Key("clear-lyric-cache"), "清理缓存").Height(40).FontSize(12).
					Disabled(lyrics.cacheBytes == 0 && lyrics.cacheSizeErr == nil && lyrics.cacheClearErr == nil).Clicked() {
					lyrics.cacheVersion++
					active := lyrics.cache[lyrics.song]
					clear(lyrics.cache)
					if len(active) > 0 {
						lyrics.cache[lyrics.song] = active
					}
					lyrics.cacheClearErr = lyrics.diskCache.Clear()
					a.refreshLyricCacheSize()
					if lyrics.cacheClearErr != nil {
						c.Announce("歌词缓存清理失败，可重试")
					} else {
						c.Announce("歌词缓存已清理")
					}
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
		settingsGroup(c.Key("app-settings"), "应用", func() {
			if supportsFullscreenHiding {
				settingsRow(c.Key("fullscreen-settings")).Children(func() {
					const label = "全屏时隐藏灵动岛"
					ui.Text(c, label).Grow(1).FontSize(13).TextColor(ink).AccessHidden(true)
					fullscreenChanged = ui.Switch(c.Key("hide-in-fullscreen"), &a.settings.HideInFullscreen).Label(label).Changed()
				})
				ui.Box(c).AlignSelf(ui.Stretch).Height(1).Shrink(0).Margin(0, 14).Background(theme.Border)
			}
			settingsRow(c.Key("update-settings")).Children(func() {
				ui.Text(c, fmt.Sprintf("版本 %s", mygo.App.Version())).Grow(1).FontSize(13).TextColor(ink)
				a.updateControl(c)
			})
		})
		if (modeChanged || cardStyleChanged || lyricsChanged) && a.settingsWindow != nil {
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
		if fullscreenChanged && a.win != nil {
			a.win.Update(a.syncFullscreenVisibility)
		}
		if modeChanged || colorChanged || lyricsChanged || transitionChanged || lyricSizeChanged || lyricColorChanged || idleChanged || weatherChanged || cardStyleChanged || fullscreenChanged {
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
