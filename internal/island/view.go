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
	if !a.expanded || a.media.scrubbing || a.todos.draft != nil || a.widgetDrag.active() {
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
	grid := a.panelWidgetLayout()

	body := ui.Column(c.Key("body")).Width(panelWidth-2).Shrink(0).Padding(0, panelBodyPadding, 20).
		Opacity(opacity).Translate(0, 10*(1-opacity)).IgnoreInput(!a.expanded || opacity < 0.80)
	body.Children(func() {
		containerHeight := grid.height
		if todo > 0 {
			detailHeight := float32(a.todos.detailHeight())
			if todo == 1 && frame.open == 1 {
				detailHeight = frame.height - panelHeaderHeight - 20
			}
			containerHeight = grid.height + (detailHeight-grid.height)*todo
		} else if clipboard > 0 {
			containerHeight = grid.height + (clipboardPanelHeight-grid.height)*clipboard
		}

		gridBox := ui.Box(c.Key("widgets-grid")).FillWidth().Height(containerHeight).Shrink(0)
		gridBox.ObserveInput(func(ev ui.InputEvent) {
			if ev.Button != 0 {
				return
			}
			switch ev.Kind {
			case ui.InputPointerDown:
				if progress == 0 && !a.todos.open && !a.clipboard.open {
					a.widgetDrag.beginPress(a, grid, ev.X, ev.Y, ev.Time)
				}
			case ui.InputPointerUp:
				a.widgetDrag.endPress(a, ev.Canceled)
			}
		})
		if a.widgetDrag.pressing || a.widgetDrag.dragging {
			px, py, _ := gridBox.PointerPosition()
			if remaining := a.widgetDrag.updatePointer(a, px, py, c.Now()); remaining > 0 {
				c.After(remaining)
			}
			grid = a.panelWidgetLayout()
			if a.widgetDrag.active() {
				gridBox.Cursor(ui.CursorGrabbing)
				if containerHeight < grid.height {
					gridBox.Height(grid.height)
				}
			}
		}

		dragging := a.widgetDrag.active()
		canConfigure := progress == 0 && !a.todos.open && !a.clipboard.open && !dragging

		gridBox.Children(func() {
			if dragging {
				if target, ok := grid.slot(a.widgetDrag.id); ok {
					ui.Box(c.Key("widget-drop-slot")).Absolute().
						Left(target.rect.X).Top(target.rect.Y).Size(target.rect.W, target.rect.H).
						Radius(13).Background(c.Theme().Accent.Alpha(0.10)).
						Border(1.5, c.Theme().Accent.Alpha(0.45)).IgnoreInput(true)
				}
			}

			for _, slot := range grid.slots {
				isDragged := dragging && a.widgetDrag.id == slot.id
				dx := a.widgetDrag.pointerX - a.widgetDrag.startX
				dy := a.widgetDrag.pointerY - a.widgetDrag.startY

				switch slot.id {
				case widgetBattery, widgetMedia:
					if progress >= panelCardPreviewEnd {
						continue
					}
					rect := slot.rect
					if isDragged {
						rect = a.widgetDrag.originRect
						rect.X += dx
						rect.Y += dy
					}
					wrapper := ui.Box(c.Key("widget-"+string(slot.id))).Absolute().
						Left(rect.X).Top(rect.Y).Size(rect.W, rect.H)
					if canConfigure {
						id := slot.id
						wrapper.ContextMenu(func(m *ui.Menu) { a.widgetSizeMenu(m, id) })
					}
					if isDragged {
						wrapper.ZIndex(20).Scale(1.03, 1.03).Opacity(0.95).Radius(16).
							Border(1.5, c.Theme().Accent.Alpha(0.55))
					} else {
						wrapper.Opacity(overviewOpacity).
							IgnoreInput(progress > 0 || a.todos.open || a.clipboard.open || dragging)
					}
					wrapper.Children(func() {
						if slot.id == widgetBattery {
							a.batteryWidget(c, slot.size, rect.W, rect.H)
						} else {
							a.mediaWidget(c, slot.size, rect.W, rect.H)
						}
					})

				case widgetTodo:
					if !(todo > 0 || clipboard < panelCardPreviewEnd) {
						continue
					}
					todoFade := max(0, clipboard-todo)
					cardOpacity := 1 - animationPhase(todoFade, 0, panelCardPreviewEnd)
					left := slot.rect.X * (1 - todo)
					top := slot.rect.Y * (1 - todo)
					renderSlot := slot
					if isDragged {
						renderSlot.rect = a.widgetDrag.originRect
						left = a.widgetDrag.originRect.X + dx
						top = a.widgetDrag.originRect.Y + dy
					}
					card := a.todoCardInSlot(c, renderSlot, todo, cardOpacity, frame).
						Absolute().Left(left).Top(top)
					if canConfigure {
						card.ContextMenu(func(m *ui.Menu) { a.widgetSizeMenu(m, widgetTodo) })
					}
					if isDragged {
						card.ZIndex(20).Scale(1.03, 1.03).Opacity(0.95).
							Border(1.5, c.Theme().Accent.Alpha(0.55))
					} else if todo > 0 {
						card.ZIndex(5)
					}

				case widgetClipboard:
					if !(a.clipboard.Recording && (clipboard > 0 || todo < panelCardPreviewEnd)) {
						continue
					}
					clipboardFade := max(0, todo-clipboard)
					cardOpacity := 1 - animationPhase(clipboardFade, 0, panelCardPreviewEnd)
					left := slot.rect.X * (1 - clipboard)
					top := slot.rect.Y * (1 - clipboard)
					renderSlot := slot
					if isDragged {
						renderSlot.rect = a.widgetDrag.originRect
						left = a.widgetDrag.originRect.X + dx
						top = a.widgetDrag.originRect.Y + dy
					}
					card := a.clipboardCardInSlot(c, renderSlot, clipboard, cardOpacity, a.expanded && opacity >= 0.80).
						Absolute().Left(left).Top(top)
					if canConfigure {
						card.ContextMenu(func(m *ui.Menu) { a.widgetSizeMenu(m, widgetClipboard) })
					}
					if isDragged {
						card.ZIndex(20).Scale(1.03, 1.03).Opacity(0.95).
							Border(1.5, c.Theme().Accent.Alpha(0.55))
					} else if clipboard > 0 {
						card.ZIndex(5)
					}
				}
			}
		})
	})
	a.widgetDrag.suppressClick = false
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
	if a.expanded && a.widgetDrag.active() {
		if c.Shortcut(0, ui.KeyEscape) {
			a.widgetDrag.cancel(a)
		}
		return
	}
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
