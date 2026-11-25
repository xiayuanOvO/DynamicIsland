package island

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	panelBodyPadding      = 22
	panelBodyWidth        = panelWidth - 2 - 2*panelBodyPadding
	overviewCardGap       = 8
	overviewToolCardWidth = (panelBodyWidth - overviewCardGap) / 2
	panelCardPreviewEnd   = 0.45
	panelCardDetailEnd    = 0.9
	panelCardInputStart   = 0.85
)

func (a *island) view(c *ui.Context) {
	c.SetTheme(islandThemes[a.resolvedTheme()])
	c.Root().Background(ui.Transparent)
	frame := a.shownFrame
	w, h := int(frame.width), int(frame.height)
	root := ui.Column(c.Key("island")).MinWidth(0).MinHeight(0).Radius(islandCornerRadius(frame.height)).Clip()
	hovered := false
	if a.canvas.restore != nil {
		width, height := c.Size()
		if int(width) == a.canvas.bounds.Width && int(height) == a.canvas.bounds.Height {
			a.canvas.warming = false
		}
		bounds := a.windowBounds(w, h)
		region := a.canvas.contentRect(bounds)
		root.Absolute().Left(float32(region.X)).Top(float32(region.Y)).Size(float32(w), float32(h))
		point := mygo.Screen.CursorScreenPoint()
		hovered = point.X >= bounds.X && point.X < bounds.X+w && point.Y >= bounds.Y && point.Y < bounds.Y+h
	} else {
		root.Fill()
		hovered = pointerOverWindow(a.win, root.Hovered())
	}
	glassSurface(c, root, a.hasWindowBackdrop || c.Vibrancy(), a.canvas.restore != nil)
	a.finishMove(c)
	if !a.moving {
		clipboardFocused := a.clipboard.open && c.WindowActive() && (a.clipboard.openedByShortcut || root.FocusWithin())
		a.hover(c, hovered || clipboardFocused)
	}

	a.updateMedia(c)
	a.updateWave(c)
	a.updateBattery(c)
	a.updateIdleWeather(c)
	if !a.moving {
		a.animation.retarget(a.targetFrame(), c.Now())
		a.scheduleWindowFrame(time.Now().Add(windowFrameInterval))
	}
	a.updateLyrics(c, frame, w, h)
	root.ContextMenu(a.menu)
	root.Children(func() {
		a.header(c, frame.open)
		if a.expanded || h > compactHeight+1 {
			a.body(c, frame)
		}
	})
	a.shortcuts(c)
}

func (a *island) hover(c *ui.Context, hovered bool) {
	if hovered {
		a.leaveSince = time.Time{}
		return
	}
	if !a.expanded || a.media.scrubbing || a.todos.draft != nil {
		return
	}
	now := c.Now()
	if a.leaveSince.IsZero() {
		a.leaveSince = now
	}
	remaining := 200*time.Millisecond - now.Sub(a.leaveSince)
	if remaining <= 0 {
		a.setExpanded(false)
	} else {
		c.After(remaining)
	}
}

func (a *island) header(c *ui.Context, open float32) {
	docked := a.dock.side != dockNone
	idle := a.compactIdle()
	track := a.media.track
	playing := track.Playing && track.Title != ""
	now := c.Now()
	alertActive, alertProgress, alertExiting := false, float32(0), false
	if !docked && !a.expanded {
		alertActive, alertProgress, alertExiting = a.activeCapsuleAlert(now, c.Preferences().ReduceMotion)
	}
	height := float32(compactHeight)
	if !docked && (idle || playing || alertActive) {
		height = idleCapsuleHeight
	}
	height += (panelHeaderHeight - height) * open
	offset := -float32(a.dock.side) * (dockTuck / 2.0) * (1 - open)
	compactAlpha := 1 - animationPhase(open, 0, 0.6)
	baseAlpha := compactAlpha * (1 - alertProgress)
	baseShiftY := -7 * alertProgress
	showIdle := baseAlpha > 0.01 && idle
	showFace := showIdle && a.settings.showsIdleItem(idleItemFace)
	if !showFace {
		a.face = idleFaceState{}
	}
	header := ui.Row(c.Key("header")).FillWidth().Height(height).Shrink(0).
		Padding(0, 22).Gap(8).AlignItems(ui.Center)
	if a.expanded {
		header.DragWindow().Cursor(ui.CursorGrab)
	} else {
		header.HandleInput(a.capsuleInput).Cursor(ui.CursorPointer).Focusable().Role(ui.RoleButton).Label("展开灵动岛")
		press := a.press
		// MyGo can deliver the release a frame before it reports the click.
		if _, held := header.PressedButton(); press != nil && press.released && !held {
			a.press = nil
		}
		if header.Clicked() && (press == nil || (press.released && !press.moved)) {
			a.setExpanded(true)
		}
	}
	header.Children(func() {
		if openAlpha := animationPhase(open, 0.35, 0.9); openAlpha > 0.01 {
			btn := iconButton(c.Key("open-settings"), settingsIcon, "设置").
				Absolute().Right(14).Top((height - 20) / 2).Size(20, 20).Radius(6).
				Opacity(openAlpha).IgnoreInput(!a.expanded || open < 0.8)
			if btn.Clicked() {
				a.openSettings()
			}
		}
		if compactAlpha > 0 && alertActive {
			if !c.Preferences().ReduceMotion {
				c.AnimationFrame()
			} else if remaining := a.alert.until.Sub(now); remaining > 0 {
				c.After(remaining)
			}
			if alertProgress > 0.01 {
				a.capsuleAlertView(c, compactAlpha*alertProgress, alertProgress, alertExiting)
			}
		}
		if showIdle {
			if docked {
				if showFace {
					slot := ui.Box(c.Key("idle-face")).Absolute().Left(0).Right(0).Top(0).Height(compactHeight).
						Translate(offset, 0).Opacity(baseAlpha)
					a.drawIdleFace(c, slot, baseAlpha, offset)
				}
			} else {
				a.idleCapsule(c, baseAlpha, baseShiftY)
			}
			return
		}
		if baseAlpha > 0.01 && playing {
			const coverSize, timeWidth, waveHeight = 26, 42, 20
			mediaHeight := float32(idleCapsuleHeight)
			mediaWidth, waveInset, bandsPerBar := float32(compactMediaWidth-20), float32(46), 4
			if docked {
				mediaHeight = compactHeight
				mediaWidth, waveInset, bandsPerBar = 18, 0, 13
			} else {
				c.After(200 * time.Millisecond)
			}
			ui.Box(c.Key("compact-media")).Absolute().Left(0).Right(0).Top(0).Height(mediaHeight).
				Opacity(baseAlpha).Translate(offset, baseShiftY).IgnoreInput(true).Center().Children(func() {
				ui.Box(c).Size(mediaWidth, mediaHeight).Shrink(0).Children(func() {
					ui.Box(c.Key("compact-wave")).Absolute().Left(waveInset).Right(waveInset).
						Top((mediaHeight - waveHeight) / 2).Height(waveHeight).Label("播放律动").
						Draw(func(p *ui.Painter, r ui.Rect) {
							paintMediaBars(p, r, a.media.wave[:], bandsPerBar, 1, a.settings.waveColor(a.media.artworkColor, c.Theme().Dark))
						})
					if !docked {
						a.media.cover(c.Key("compact-cover"), coverSize).
							Absolute().Left(0).Top((mediaHeight - coverSize) / 2)
						ui.Text(c.Key("compact-time"), mediaTime(track.Elapsed(now))).
							Absolute().Right(0).Top((mediaHeight-16)/2).Size(timeWidth, 16).
							SingleLine().TextAlign(ui.Center).FontSize(11.5).FontWeight(600).TextColor(c.Theme().Text.Alpha(0.72))
					}
				})
			})
		}
		if baseAlpha > 0.01 && !playing {
			item := a.todos.next()
			if item == nil {
				return
			}
			width := float32(compactWidth)
			if docked {
				width = compactHeight
			}
			ui.Box(c.Key("compact-summary")).Absolute().Top(0).Left(0).Right(0).Height(compactHeight).
				Opacity(compactAlpha).Translate(offset, 0).IgnoreInput(true).Center().Children(func() {
				row := ui.Row(c).Size(width, compactHeight).Shrink(0).Gap(8).AlignItems(ui.Center)
				if docked {
					row.Justify(ui.Center)
				} else {
					row.Padding(0, 22)
				}
				row.Children(func() {
					ui.Icon(c, todoIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().Accent)
					if !docked {
						ui.Text(c, item.Title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12).TextColor(c.Theme().Text)
					}
				})
			})
		}
	})
}

func (a *island) body(c *ui.Context, frame islandFrame) {
	opacity := frame.content
	todo, clipboard := frame.todo, frame.clipboard
	if c.Preferences().ReduceMotion {
		todo, clipboard = 0, 0
		if a.todos.open {
			todo = 1
		}
		if a.clipboard.open {
			clipboard = 1
		}
	}
	if !a.clipboard.Recording {
		clipboard = 0
	}
	progress := max(todo, clipboard)
	overviewOpacity := 1 - animationPhase(progress, 0, panelCardPreviewEnd)
	leadHeight := a.overviewLeadHeight()
	body := ui.Column(c.Key("body")).Width(panelWidth-2).Shrink(0).Padding(0, panelBodyPadding, 20).
		Opacity(opacity).Translate(0, 10*(1-opacity)).IgnoreInput(!a.expanded || opacity < 0.80)
	body.Children(func() {
		if progress < 1 && leadHeight > 0 {
			// Keep the battery row and music card whole as their slot shrinks.
			lead := ui.Column(c.Key("overview-lead")).FillWidth().Shrink(0).
				Height(float32(leadHeight) * (1 - progress))
			if overviewOpacity > 0 {
				lead.ZIndex(1).Opacity(overviewOpacity).
					IgnoreInput(progress > 0 || a.todos.open || a.clipboard.open).Children(func() {
					if len(a.battery.readings) > 0 {
						a.batteryRow(c)
					}
					if a.media.track.Title != "" {
						a.mediaCard(c)
					}
				})
			}
		}
		tools := ui.Box(c.Key("tools")).FillWidth().Shrink(0)
		toolsPreview := float32(a.toolsPreviewHeight())
		showClipboard := a.clipboard.Recording && (clipboard > 0 || todo < panelCardPreviewEnd)
		clipboardHeight := toolsPreview + (clipboardPanelHeight-toolsPreview)*clipboard
		if showClipboard {
			tools.MinHeight(clipboardHeight)
		}
		tools.Children(func() {
			// Counter the shrinking overview slot for the card that is only fading out.
			todoFade := max(0, clipboard-todo)
			clipboardFade := max(0, todo-clipboard)
			if todo > 0 || clipboard < panelCardPreviewEnd {
				a.todoCard(c, todo, 1-animationPhase(todoFade, 0, panelCardPreviewEnd), frame).
					Translate(0, float32(leadHeight)*todoFade)
			}
			if showClipboard {
				a.clipboardCard(c, clipboard, toolsPreview, clipboardHeight, 1-animationPhase(clipboardFade, 0, panelCardPreviewEnd),
					a.expanded && opacity >= 0.80).Translate(0, float32(leadHeight)*clipboardFade)
			}
		})
	})
}

func panelCardSurface(c *ui.Context, progress, previewWidth, height float32) ui.Element {
	surface := ui.Box(c).Size(previewWidth+(panelBodyWidth-previewWidth)*progress, height).Shrink(0)
	if progress < 1 {
		tint := colorsFor(c.Theme().Dark).glass
		surface.Radius(13).Clip().Background(tint.Alpha(0.4*(1-progress))).
			Border(0.75, tint.Alpha(0.65*(1-progress)))
	}
	return surface
}

func (a *island) menu(m *ui.Menu) {
	if !a.settings.HideLyrics && a.media.track.Playing && a.media.track.Title != "" {
		song := songForTrack(a.media.track)
		offset := a.lyricOffset(song)
		if m.Item("歌词提前 0.5 秒").Disabled(offset >= lyricOffsetMax).Chosen() {
			a.setLyricOffset(song, offset+lyricOffsetStep)
		}
		if m.Item("歌词延后 0.5 秒").Disabled(offset <= -lyricOffsetMax).Chosen() {
			a.setLyricOffset(song, offset-lyricOffsetStep)
		}
		if offset != 0 && m.Item("重置歌词偏移 ("+formatLyricOffset(offset)+")").Chosen() {
			a.setLyricOffset(song, 0)
		}
		m.Separator()
	}
	if m.Item("回到屏幕顶部").Chosen() {
		a.recenter()
	}
	if m.Item("隐藏灵动岛").Chosen() {
		a.hide()
	}
	m.Separator()
	if m.Item("设置…").Chosen() {
		a.win.Update(a.openSettings)
	}
	if m.Item("退出").Chosen() {
		mygo.App.Quit()
	}
}

func (a *island) shortcuts(c *ui.Context) {
	if a.expanded && a.clipboard.open {
		a.clipboardShortcuts(c)
		return
	}
	if a.expanded && a.todos.open {
		if c.Shortcut(0, ui.KeyEscape) {
			if a.todos.draft != nil {
				a.todos.closeEditor()
			} else {
				a.todos.open = false
			}
		}
		return
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.setExpanded(false)
	}
}

func iconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	button := ui.ButtonBase(c).Size(26, 26).Shrink(0).Radius(8).Center().
		TextColor(c.Theme().TextMuted).Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(c.Theme().SurfaceHover).TextColor(c.Theme().Text)
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(15, 15)
	})
	return button
}
