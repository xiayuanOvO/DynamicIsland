package island

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const clipboardPanelHeight = 284

var (
	clearClipboardIcon = svg(`<path d="M4 6h16M4 12h9M4 18h9m3-5 5 5m0-5-5 5"/>`)
	pinIcon            = svg(`<path d="M12 17v5M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"/>`)
	pinnedIcon         = svg(`<path d="M12 17v5"/><path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z" fill="currentColor"/>`)
)

func (a *island) openClipboard(fromShortcut bool) {
	if !a.clipboard.Recording {
		return
	}
	a.todos.open = false
	a.todos.draft, a.todos.leaving = nil, nil
	history := &a.clipboard
	if !history.open {
		history.query = ""
		a.refreshClipboardEntries(time.Now(), true)
		history.confirmClear, history.pendingPaste = false, nil
		history.pasteTarget = clipboardPasteTarget{}
	}
	if a.win != nil && (!history.open || fromShortcut) {
		if target := captureClipboardPasteTarget(); target != (clipboardPasteTarget{}) {
			history.pasteTarget = target
		}
	}
	history.open, history.focusSearch = true, true
	history.openedByShortcut = fromShortcut
	a.showPanel()
}

func (a *island) clipboardSettings(c *ui.Context) {
	history := &a.clipboard
	row := settingsRow(c.Key("clipboard-recording"))
	row.Children(func() {
		if history.loadFailed {
			ui.Text(c, history.problem).Grow(1).FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem)
			confirm := ui.Local(row, "confirm-reset", func() bool { return false })
			if ui.Button(c.Key("reset-history"), "清空历史…").Height(40).Clicked() {
				*confirm = true
			}
			if ui.AlertDialog(c, confirm, "清空剪贴板历史？", "", "清空历史", "取消") == 0 &&
				a.resetClipboard() {
				c.Announce("剪贴板历史已清空")
			}
			return
		}
		var label ui.Element
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			label = ui.Text(c, "记录剪贴板").FontSize(13).TextColor(c.Theme().Text)
			if history.problem != "" {
				ui.Text(c, history.problem).FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem)
			}
		})
		recording := history.Recording
		if ui.Switch(c.Key("record"), &recording).LabelledBy(label).Changed() {
			next := history.clipboardState
			next.Recording = recording
			if !a.saveClipboardSettings(next) {
				return
			}
			a.syncClipboardRecording()
			if !recording {
				history.open, history.confirmClear = false, false
			}
			if a.tray != nil {
				a.tray.SetMenu(a.trayMenu())
			}
			if a.win != nil {
				a.win.Invalidate()
			}
		}
	})
	if !history.loadFailed {
		settingsDivider(c)
		a.clipboardShortcutSettings(c)
	}
}

func (a *island) clipboardCard(c *ui.Context, progress, height, opacity float32, interactive bool) ui.Element {
	surface := panelCardSurface(c.Key("clipboard-surface"), progress, panelBodyWidth/2-8, height).
		Absolute().Top(0).Right(0).Label("剪贴板卡片").Opacity(opacity).
		IgnoreInput(a.todos.open || opacity < panelCardInputStart)
	surface.Children(func() {
		if progress < panelCardPreviewEnd {
			card := ui.ButtonBase(c.Key("clipboard-card")).Column().Absolute().Top(0).Left(0).Right(0).
				Height(todoPreviewMinHeight).Padding(todoPreviewPadding, 10).AlignItems(ui.Stretch).Justify(ui.Start).
				Radius(13).Cursor(ui.CursorPointer).Label("查看剪贴板历史").
				Opacity(1 - animationPhase(progress, 0, panelCardPreviewEnd)).IgnoreInput(a.clipboard.open)
			if card.Hovered() {
				card.Background(colorsFor(c.Theme().Dark).glass.Alpha(0.2))
			}
			card.Children(func() {
				ui.Row(c).Height(todoPreviewHeader).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, clipboardIcon).Size(14, 14).TextColor(c.Theme().Accent)
					ui.Text(c, "剪贴板历史").SingleLine().FontSize(11).FontWeight(600).TextColor(c.Theme().TextMuted)
				})
			})
			if card.Clicked() {
				a.openClipboard(false)
			}
		}
		if progress > panelCardPreviewEnd {
			interactive = interactive && a.clipboard.open && progress >= panelCardInputStart
			ui.Column(c.Key("clipboard-detail")).Absolute().Top(0).Left(0).Right(0).
				Height(clipboardPanelHeight).Opacity(animationPhase(progress, panelCardPreviewEnd, panelCardDetailEnd)).
				IgnoreInput(!interactive).Children(func() { a.clipboardView(c, interactive, writeClipboardEntry) })
		}
	})
	return surface
}

func (a *island) clipboardView(c *ui.Context, interactive bool, write func(transfer.Data) error) {
	history := &a.clipboard
	ui.Themed(c.Key("clipboard-history"), panelTheme(c)).FillWidth().Height(clipboardPanelHeight).
		Shrink(0).Gap(8).Background(ui.Transparent).Children(func() {
		var search ui.Element
		if !history.confirmClear {
			ui.Row(c.Key("toolbar")).FillWidth().Height(40).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
				if iconButton(c.Key("back"), backIcon, "返回岛面板").Size(40, 40).Clicked() {
					history.open, history.confirmClear = false, false
				}
				search = ui.SearchField(c.Key("search"), &history.query).Grow(1).MinWidth(0).Height(36).
					Label("搜索剪贴板历史").Placeholder("搜索记录")
				if history.hasEntries && iconButton(c.Key("clear"), clearClipboardIcon, "清空历史").Size(40, 40).Clicked() {
					history.confirmClear = true
				}
			})
		}
		if history.confirmClear {
			ui.Column(c.Key("confirm-clear")).FillWidth().Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "清空所有剪贴板历史？").FontSize(13).TextColor(c.Theme().Text)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c.Key("cancel"), "取消").Height(40).AutoFocus().Clicked() {
						history.confirmClear = false
						history.focusSearch = true
					}
					if ui.Button(c.Key("confirm"), "清空历史").Height(40).Clicked() {
						if err := history.store.clear(); err != nil {
							log.Printf("clear clipboard history: %v", err)
							history.problem = "清空历史失败"
						} else {
							a.refreshClipboardEntries(c.Now(), true)
							c.Invalidate()
							history.confirmClear = false
							history.focusSearch = true
							c.Announce("剪贴板历史已清空")
						}
					}
				})
			})
		} else {
			reveal := history.focusSearch && interactive
			if reveal {
				search.Focus()
				history.focusSearch = false
			}
			if search.Changed() {
				a.refreshClipboardEntries(c.Now(), true)
				reveal = true
			} else if history.hoverSelection != 0 {
				history.selected = history.hoverSelection
			}
			history.hoverSelection = 0
			entries := history.entries
			selected := history.selectedIndex()
			if len(entries) == 0 {
				empty := "暂无剪贴板历史"
				if strings.TrimSpace(history.query) != "" {
					empty = "未找到匹配记录"
				}
				ui.Box(c).FillWidth().Grow(1).Center().Children(func() {
					ui.Text(c, empty).FontSize(13).TextColor(c.Theme().TextMuted)
				})
			} else {
				entry := entries[selected]
				history.selected = entry.ID
				if interactive && history.pendingPaste == nil && (search.Submitted() || c.Shortcut(0, ui.KeyEnter)) {
					history.pendingPaste = &entry
				}
				if reveal {
					history.list.ScrollIntoView(selected)
				}
				history.list.EndReached = nil
				if interactive && history.hasMore {
					now := c.Now()
					history.list.NearEnd = 2
					history.list.EndReached = func(int) { a.loadMoreClipboardEntries(now) }
				}
				list := ui.List(c.Key("entries"), &history.list, len(entries)).
					ItemKey(func(index int) any { return entries[index].ID }).FillWidth().Grow(1).MinHeight(0).Gap(3).
					HandleInput(func(event ui.InputEvent) bool {
						if event.Kind == ui.InputScroll {
							// MyGo updates the pointer before dispatching scroll input.
							// Discard the hover staged by that same scroll event.
							history.hoverSelection = 0
						}
						return false
					})
				list.Rows(func(item ui.ListRow) {
					row := a.clipboardRow(item.Context, entries[item.Index], item.Index == selected, search, write)
					if item.Index == selected {
						search.ActiveDescendant(row)
					}
				})
			}
			// Release Enter while the island is still focused so a held key
			// cannot repeat into the original input after focus is restored.
			if interactive && history.pendingPaste != nil && !c.KeyDown(ui.KeyEnter) {
				entry := *history.pendingPaste
				history.pendingPaste = nil
				a.useClipboardEntry(c, entry, true, write)
			}
		}
		if history.problem != "" {
			ui.Text(c, history.problem).FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem).SingleLine()
		}
	})
}

func (a *island) clipboardRow(c *ui.Context, entry clipboardEntry, selected bool, search ui.Element, write func(transfer.Data) error) ui.Element {
	text := entry.Preview
	dimensions := ""
	const rowHeight = 50
	if entry.Width != 0 {
		if text == "" {
			text = "图片"
		}
		dimensions = fmt.Sprintf("%d × %d", entry.Width, entry.Height)
	}
	var copy ui.Element
	key := entry.ID
	history := &a.clipboard
	row := ui.Row(c.Key(key)).FillWidth().Height(rowHeight).Shrink(0).Gap(2).AlignItems(ui.Center).Radius(10).
		HandleInput(func(event ui.InputEvent) bool {
			if event.Kind != ui.InputPointerMove || event.Button != -1 {
				return false
			}
			history.hoverSelection = key
			return history.selected != key
		})
	if selected {
		row.Background(c.Theme().SurfaceHover)
	}
	row.Children(func() {
		label := text
		if dimensions != "" {
			label += " " + dimensions
		}
		copy = ui.ButtonBase(c.Key("copy")).Row().Grow(1).MinWidth(0).Height(rowHeight).Padding(0, 9).
			Gap(8).AlignItems(ui.Center).Radius(8).
			TextColor(c.Theme().TextMuted).Cursor(ui.CursorPointer).Label("复制 " + label)
		copy.Children(func() {
			preview := ui.Box(c.Key("preview")).Size(44, 34).Shrink(0).Radius(6).Clip().Center()
			if entry.Width != 0 {
				preview.Background(colorsFor(c.Theme().Dark).glass.Alpha(0.45))
			}
			preview.Children(func() {
				if entry.Width == 0 {
					ui.Icon(c, copyIcon).Size(14, 14)
					return
				}
				if copy.InViewMargin(rowHeight) {
					visual := a.clipboardPreview(entry.ID)
					if visual != nil && visual.source != nil {
						ui.Image(c, visual.source).Fill().Fit(ui.Contain)
						return
					}
				}
				ui.Icon(c, imageIcon).Size(16, 16).TextColor(c.Theme().TextMuted)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, text).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13).TextColor(c.Theme().Text)
				if dimensions != "" {
					ui.Text(c, dimensions).FontSize(10).TextColor(c.Theme().TextMuted)
				}
			})
			ui.Text(c, entry.CopiedAt.Local().Format("01-02 15:04")).Shrink(0).FontSize(10).TextColor(c.Theme().TextMuted)
		})
		if copy.Clicked() {
			if c.KeyDown(ui.KeyEnter) {
				if a.clipboard.pendingPaste == nil {
					a.clipboard.pendingPaste = &entry
				}
			} else {
				a.useClipboardEntry(c, entry, false, write)
			}
		}
		pinTooltip, pinAction, icon := "置顶", "置顶记录 ", pinIcon
		if entry.Pinned {
			pinTooltip, pinAction, icon = "取消置顶", "取消置顶记录 ", pinnedIcon
		}
		pin := iconButton(c.Key("pin"), icon, pinTooltip).Size(34, 34).Label(pinAction + label)
		if entry.Pinned {
			pin.TextColor(c.Theme().Accent).Background(c.Theme().Accent.Alpha(0.16))
		}
		if pin.Clicked() {
			index := slices.IndexFunc(history.entries, func(item clipboardEntry) bool { return item.ID == entry.ID })
			if err := history.store.setPinned(entry.ID, !entry.Pinned); err != nil {
				log.Printf("pin clipboard entry: %v", err)
				history.problem = "置顶状态保存失败"
			} else {
				a.refreshClipboardEntries(c.Now(), false)
				if index >= 0 && index < len(history.entries) {
					history.selected = history.entries[index].ID
				}
				search.Focus()
				c.Invalidate()
			}
		}
		if iconButton(c.Key("remove"), trashIcon, "删除此条记录").Size(34, 34).Label("删除记录 " + label).Clicked() {
			if err := history.store.deleteEntry(entry.ID); err != nil {
				log.Printf("delete clipboard entry: %v", err)
				history.problem = "删除记录失败"
			} else {
				a.refreshClipboardEntries(c.Now(), false)
				c.Announce("已删除剪贴板记录")
			}
			c.Invalidate()
		}
	})
	return copy
}

func writeClipboardEntry(data transfer.Data) error {
	if err := mygo.Clipboard.Write(data); err != nil {
		return err
	}
	if err := mygo.Clipboard.Flush(); err != nil {
		log.Printf("flush clipboard: %v", err)
	}
	return nil
}

func (a *island) useClipboardEntry(c *ui.Context, entry clipboardEntry, paste bool, write func(transfer.Data) error) {
	history := &a.clipboard
	data, err := a.clipboardData(entry)
	if err != nil {
		history.problem = "剪贴板记录读取失败"
		return
	}
	if err := write(data); err != nil {
		history.problem = "复制失败，可重试"
		return
	}
	history.problem = ""
	if !paste {
		c.Toast("已复制")
		return
	}
	target := history.pasteTarget
	a.setExpanded(false)
	if win := a.win; win != nil {
		win.Update(func() {
			if a.expanded || a.hiddenByUser || a.hiddenForFullscreen {
				return
			}
			pasteReady := target.restore(win)
			if !win.IsVisible() {
				frame := a.targetFrame()
				a.animation = islandAnimation{from: frame, to: frame, started: time.Now()}
				a.applyWindowFrame(frame, false)
				win.ShowInactive()
			}
			if !pasteReady {
				return
			}
			// Desktop focus changes are delivered asynchronously.
			time.AfterFunc(75*time.Millisecond, func() {
				mygo.RunOnMain(func() {
					if !a.expanded && history.Recording {
						target.paste()
					}
				})
			})
		})
	}
}
