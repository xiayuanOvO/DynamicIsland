package island

import (
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	settingsPageIdle      = "idle"
	settingsPageMedia     = "media"
	settingsPageClipboard = "clipboard"
	settingsPageTheme     = "theme"
	settingsPageApp       = "app"
	settingsSidebarWidth  = 138
	settingsPageInset     = 24
	settingsControlWidth  = 222
	settingsRowHeight     = 46
	cardPreviewInset      = 8
	cardPreviewGap        = 12
)

var settingsPages = [...]struct {
	id, name string
	icon     *ui.SVG
}{
	{settingsPageIdle, "空闲", svg(`<rect x="3" y="6" width="18" height="12" rx="6"/><path d="M9 11v2m6-2v2"/>`)},
	{settingsPageMedia, "媒体", svg(`<path d="M9 18V6l11-3v12M9 9l11-3"/><ellipse cx="6" cy="18" rx="3" ry="2.5"/><ellipse cx="17" cy="15" rx="3" ry="2.5"/>`)},
	{settingsPageClipboard, "剪贴板", clipboardIcon},
	{settingsPageTheme, "主题", svg(`<circle cx="12" cy="12" r="8"/><path d="M12 4v16m0 0a8 8 0 0 0 0-16" fill="currentColor"/>`)},
	{settingsPageApp, "应用", svg(`<rect x="3" y="4" width="18" height="16" rx="3"/><path d="M3 9h18m-14 5h5m-5 3h9"/>`)},
}

func settingsCard(c *ui.Context, children func()) {
	ui.Column(c).FillWidth().Shrink(0).Radius(12).Background(colorsFor(c.Theme().Dark).settingsCard).
		Border(1, c.Theme().Border).Children(children)
}

func settingsSection(c *ui.Context, label string, children func()) {
	ui.Column(c).FillWidth().Shrink(0).Gap(6).Children(func() {
		ui.Text(c, label).Padding(0, 3).FontSize(12).FontWeight(650).TextColor(c.Theme().TextMuted).Role(ui.RoleHeading).Level(2)
		settingsCard(c, children)
	})
}

func settingsRow(c *ui.Context) ui.Element {
	return ui.Row(c).FillWidth().Height(settingsRowHeight).Shrink(0).Padding(0, 14).Gap(12).AlignItems(ui.Center)
}

func settingsDivider(c *ui.Context) {
	ui.Box(c).AlignSelf(ui.Stretch).Height(1).Shrink(0).Margin(0, 14).Background(c.Theme().Border)
}

func (a *island) settingsView(c *ui.Context) {
	settingsTheme := settingsThemes[a.resolvedTheme()]
	c.SetTheme(settingsTheme)
	c.Root().Background(settingsTheme.Background)
	if a.settingsPage == "" {
		a.settingsPage = settingsPageIdle
	}
	if a.updates.announcement != "" {
		c.Announce(a.updates.announcement)
		a.updates.announcement = ""
	}

	ui.Row(c.Key("settings")).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Sidebar(c.Key("settings-sidebar"), &a.settingsPage, func() {
			for _, page := range settingsPages {
				ui.SidebarItem(c, page.id, page.icon, page.name).Height(40).Cursor(ui.CursorPointer)
			}
		}).Width(settingsSidebarWidth).FillHeight().Padding(18, 10).Gap(4).
			Background(colorsFor(c.Theme().Dark).sidebar).Label("设置分类")
		if a.settingsPage != settingsPageIdle {
			a.citySearch.open = false
		}
		if a.settingsPage != settingsPageClipboard {
			a.clipboard.editingShortcut, a.clipboard.shortcutProblem = false, ""
		}
		if a.settingsPage != settingsPageApp {
			a.editingHotkey, a.hotkeyProblem = false, ""
			a.openAtLoginErr = ""
			a.showLogs, a.logClearErr = false, ""
		}
		ui.Box(c).Width(1).Shrink(0).FillHeight().Background(c.Theme().Border)
		ui.Column(c.Key("settings-content")).Grow(1).FillHeight().MinWidth(0).Children(func() {
			for _, page := range settingsPages {
				if page.id == a.settingsPage {
					ui.Text(c.Key("settings-page-title"), page.name).Padding(20, settingsPageInset, 16).
						FontSize(22).FontWeight(680).TextColor(c.Theme().Text).Role(ui.RoleHeading).Level(1)
					break
				}
			}
			changed := false
			ui.Scroll(c.Key(a.settingsPage)).FillWidth().Grow(1).MinHeight(0).Padding(0, settingsPageInset, 20).Gap(16).Children(func() {
				switch a.settingsPage {
				case settingsPageIdle:
					settingsSection(c.Key("idle-display-settings"), "显示项目", func() { changed = a.idleDisplaySettings(c) })
					settingsCard(c.Key("idle-weather-settings"), func() {
						if a.weatherSettings(c) {
							changed = true
						}
						settingsDivider(c)
						settingsRow(c.Key("weather-source-settings")).Children(func() {
							const label = "天气源"
							ui.Text(c, label).Grow(1).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
							if ui.Segmented(c.Key("weather-source"), &a.settings.WeatherSource, "Open-Meteo", "wttr.in").
								FillEqually().Width(settingsControlWidth).Height(32).FontSize(12).Label(label).Changed() {
								a.idle.refreshAt = time.Time{}
								changed = true
							}
						})
					})
					settingsCard(c.Key("idle-battery-settings"), func() {
						settingsRow(c.Key("low-battery-alert-settings")).Children(func() {
							const label = "蓝牙低电量提醒"
							ui.Text(c, label).Grow(1).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
							if ui.Switch(c.Key("low-battery-alert"), &a.settings.LowBatteryAlert).Label(label).Changed() {
								changed = true
							}
						})
					})
				case settingsPageMedia:
					changed = a.mediaSettings(c)
				case settingsPageClipboard:
					settingsCard(c.Key("clipboard-settings"), func() { a.clipboardSettings(c) })
				case settingsPageTheme:
					settingsCard(c.Key("theme-settings"), func() {
						settingsRow(c).Children(func() {
							const label = "外观"
							ui.Text(c, label).Grow(1).FontSize(13).AccessHidden(true)
							if ui.Segmented(c.Key("appearance"), &a.settings.Theme, "浅色", "深色", "跟随系统").
								FillEqually().Width(246).Height(36).FontSize(12).Label(label).Changed() {
								changed = true
								if a.settingsWindow != nil {
									a.applyTheme()
								}
								c.AnimationFrame()
							}
						})
					})
				case settingsPageApp:
					settingsCard(c.Key("app-settings"), func() {
						settingsRow(c.Key("launch-at-login-settings")).Children(func() {
							const label = "开机启动"
							ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
								ui.Text(c, label).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
								if a.openAtLoginErr != "" {
									ui.Text(c, a.openAtLoginErr).FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem)
								}
							})
							if ui.Switch(c.Key("open-at-login"), &a.openAtLogin).Label(label).Changed() {
								a.applyOpenAtLogin(a.openAtLogin)
							}
						})
						settingsDivider(c)
						if supportsFullscreenHiding {
							settingsRow(c.Key("fullscreen-settings")).Children(func() {
								const label = "全屏时隐藏灵动岛"
								ui.Text(c, label).Grow(1).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
								if ui.Switch(c.Key("hide-in-fullscreen"), &a.settings.HideInFullscreen).Label(label).Changed() {
									changed = true
									if a.win != nil {
										a.win.Update(a.syncFullscreenVisibility)
									}
								}
							})
							settingsDivider(c)
						}
						a.toggleShortcutSettings(c)
						settingsDivider(c)
						settingsRow(c.Key("update-settings")).Children(func() {
							ui.Column(c).Grow(1).Gap(1).Children(func() {
								ui.Text(c, "软件更新").FontSize(13).TextColor(c.Theme().Text)
								if version := mygo.App.Version(); version != "" {
									ui.Text(c, "版本 "+version).FontSize(11).TextColor(c.Theme().TextMuted)
								}
							})
							a.updateControl(c)
						})
						settingsDivider(c)
						a.appLogSettings(c)
					})
				}
			})
			if changed {
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
				ui.Text(c, "设置保存失败").Padding(6, settingsPageInset, 12).FontSize(12).TextColor(colorsFor(c.Theme().Dark).lowBattery)
			}
		})
	})
}

func (a *island) mediaSettings(c *ui.Context) bool {
	changed := false
	settingsSection(c.Key("music-card-settings"), "音乐卡片", func() {
		settingsRow(c.Key("card-style")).Children(func() {
			ui.Text(c, "风格").Grow(1).FontSize(13).TextColor(c.Theme().Text)
			if ui.Segmented(c.Key("card-mode"), &a.settings.MediaCardStyle, "默认风格", "大封面").
				FillEqually().Width(settingsControlWidth).Height(30).FontSize(12).Label("音乐卡片选项").Changed() {
				changed = true
			}
		})
		ui.Column(c.Key("card-preview-area")).FillWidth().Padding(0, 14, cardPreviewGap).Children(func() {
			ui.Column(c.Key("card-preview")).FillWidth().Padding(cardPreviewInset).
				Radius(12).Background(colorsFor(c.Theme().Dark).mediaPreview).Label("音乐卡片预览").
				Children(func() { a.mediaCardPreview(c) })
		})
	})
	settingsSection(c.Key("lyrics-settings"), "歌词", func() {
		settingsRow(c.Key("lyric-settings")).Children(func() {
			ui.Text(c, "显示歌词").Grow(1).FontSize(13).TextColor(c.Theme().Text)
			shown := !a.settings.HideLyrics
			if ui.Switch(c.Key("show-lyrics"), &shown).Label("歌词显示").Changed() {
				a.settings.HideLyrics = !shown
				changed = true
			}
		})
		if !a.settings.HideLyrics {
			style := &a.settings.LyricStyle
			settingsDivider(c)
			settingsRow(c.Key("lyric-size-settings")).Children(func() {
				ui.Text(c, "歌词字号").Grow(1).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
				ui.Row(c).Width(settingsControlWidth).Gap(8).AlignItems(ui.Center).Children(func() {
					size := float64(style.fontSize())
					if ui.Slider(c.Key("lyric-font-size"), &size, lyricMinFontSize, lyricMaxFontSize).
						Step(1).Grow(1).Height(24).Label("歌词字号").Changed() {
						style.FontSize = int(size)
						changed = true
					}
					ui.Text(c, fmt.Sprint(style.fontSize())).Width(20).FontSize(12).TextAlign(ui.End).TextColor(c.Theme().TextMuted).AccessHidden(true)
				})
			})
			settingsDivider(c)
			settingsRow(c.Key("lyric-color-settings")).Children(func() {
				ui.Text(c, "歌词高亮颜色").Grow(1).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
				color := style.highlightColor()
				if color != white && ui.Button(c.Key("reset-lyric-color"), "恢复默认").Height(36).FontSize(12).
					Label("恢复默认歌词颜色").Clicked() {
					style.Color, color = nil, white
					changed = true
				}
				if ui.ColorWell(c.Key("lyric-highlight-color"), &color).Height(36).Label("歌词高亮颜色").Changed() {
					style.Color = &color
					changed = true
				}
			})
			settingsDivider(c)
			settingsRow(c.Key("lyric-transition-settings")).Children(func() {
				ui.Text(c, "歌词过渡").Grow(1).FontSize(13).TextColor(c.Theme().Text)
				if ui.Segmented(c.Key("lyric-transition"), &a.settings.LyricTransition, "上移", "淡入", "缩放", "关闭").
					FillEqually().Width(settingsControlWidth).Height(36).FontSize(12).Label("歌词过渡").Changed() {
					changed = true
				}
			})
		}
		settingsDivider(c)
		settingsRow(c.Key("lyric-cache")).Children(func() {
			lyrics := &a.lyrics
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "歌词缓存").FontSize(13).TextColor(c.Theme().Text)
				if lyrics.cacheClearErr != nil {
					ui.Text(c, "清理失败，可重试").FontSize(11).TextColor(colorsFor(c.Theme().Dark).lowBattery)
				}
			})
			const bytesPerMB = 1_000_000
			size := fmt.Sprintf("%.2f MB", float64(lyrics.cacheBytes)/bytesPerMB)
			if lyrics.cacheSizeErr != nil {
				size = "无法读取大小"
			} else if lyrics.cacheBytes > 0 && lyrics.cacheBytes < bytesPerMB/100 {
				size = "< 0.01 MB"
			}
			ui.Text(c, size).FontSize(12).TextColor(c.Theme().TextMuted)
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
	})
	settingsSection(c.Key("wave-color-settings"), "律动柱", func() {
		settingsRow(c.Key("wave-settings")).Children(func() {
			ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "颜色").FontSize(13).SingleLine().Shrink(0).TextColor(c.Theme().Text)
				ui.Box(c.Key("wave-preview")).Size(38, 22).Shrink(0).Label("律动预览").
					Draw(func(p *ui.Painter, r ui.Rect) {
						levels := [...]float32{0.16, 0.38, 0.62, 0.52, 0.60, 0.40, 0.14}
						paintMediaBars(p, r, levels[:], 1, 1, a.settings.waveColor(a.media.artworkColor, c.Theme().Dark))
					})
			})
			if ui.Segmented(c.Key("wave-mode"), &a.settings.Mode, "默认", "自定义", "跟随海报").
				FillEqually().Width(settingsControlWidth).Height(30).FontSize(12).Label("律动配色模式").Changed() {
				changed = true
			}
		})
		if a.settings.Mode == waveCustom {
			settingsDivider(c)
			ui.Column(c).FillWidth().Padding(10, 14, 14).Children(func() {
				if ui.ColorPicker(c.Key("wave-color"), &a.settings.Color).FillWidth().Label("自定义律动柱颜色").Changed() {
					changed = true
				}
			})
		}
	})
	return changed
}
