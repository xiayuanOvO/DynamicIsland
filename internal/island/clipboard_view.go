package island

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const clipboardPanelHeight = 284

var clearClipboardIcon = svg(`<path d="M4 6h16M4 12h9M4 18h9m3-5 5 5m0-5-5 5"/>`)

func (a *island) openClipboard(fromShortcut bool) {
	if !a.clipboard.Recording {
		return
	}
	a.todos.open = false
	a.todos.draft, a.todos.leaving = nil, nil
	history := &a.clipboard
	if !history.open {
		history.query, history.selected = "", clipboardEntryKey{}
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
			ui.Text(c, history.problem).Grow(1).FontSize(11).TextColor(ui.Hex("#a65b50"))
			confirm := ui.Local(row, "confirm-reset", func() bool { return false })
			if ui.Button(c.Key("reset-history"), "清空历史…").Height(40).Clicked() {
				*confirm = true
			}
			if ui.AlertDialog(c, confirm, "清空剪贴板历史？", "", "清空历史", "取消") == 0 &&
				a.saveClipboard(clipboardState{}) {
				c.Announce("剪贴板历史已清空")
			}
			return
		}
		var label ui.Element
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			label = ui.Text(c, "记录剪贴板").FontSize(13).TextColor(ink)
			problem := history.problem
			if problem == "" && history.shortcutUnavailable {
				problem = "剪贴板快捷键不可用"
			}
			if problem != "" {
				ui.Text(c, problem).FontSize(11).TextColor(ui.Hex("#a65b50"))
			}
		})
		recording := history.Recording
		if ui.Switch(c.Key("record"), &recording).LabelledBy(label).Changed() {
			next := history.clipboardState
			next.Recording = recording
			if !a.saveClipboard(next) {
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
}

func (a *island) clipboardCard(c *ui.Context, opacity float32) {
	const cardWidth = (panelWidth-2-44)/2 - 8
	card := ui.ButtonBase(c.Key("clipboard-card")).Column().Absolute().Top(0).Right(0).
		Size(cardWidth, todoPreviewMinHeight).Padding(todoPreviewPadding, 10).AlignItems(ui.Stretch).Justify(ui.Start).
		Radius(13).Border(0.75, white.Alpha(0.65)).Background(white.Alpha(0.4)).
		Opacity(opacity).IgnoreInput(a.todos.open || opacity < 0.85).
		Cursor(ui.CursorPointer).Label("查看剪贴板历史")
	if card.Hovered() {
		card.Background(white.Alpha(0.6))
	}
	card.Children(func() {
		ui.Row(c).Height(todoPreviewHeader).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, clipboardIcon).Size(14, 14).TextColor(accent)
			ui.Text(c, "剪贴板历史").SingleLine().FontSize(11).FontWeight(600).TextColor(muted)
		})
	})
	if card.Clicked() {
		a.openClipboard(false)
	}
}

func (a *island) clipboardView(c *ui.Context, interactive bool, write func(transfer.Data) error) {
	history := &a.clipboard
	ui.Themed(c.Key("clipboard-history"), todoTheme).FillWidth().Height(clipboardPanelHeight).
		Shrink(0).Gap(8).Background(ui.Transparent).Children(func() {
		var search ui.Element
		if !history.confirmClear {
			ui.Row(c.Key("toolbar")).FillWidth().Height(40).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
				if iconButton(c.Key("back"), backIcon, "返回岛面板").Size(40, 40).Clicked() {
					history.open, history.confirmClear = false, false
				}
				search = ui.SearchField(c.Key("search"), &history.query).Grow(1).MinWidth(0).Height(36).
					Label("搜索剪贴板历史").Placeholder("搜索记录")
				if len(history.Entries) > 0 && iconButton(c.Key("clear"), clearClipboardIcon, "清空历史").Size(40, 40).Clicked() {
					history.confirmClear = true
				}
			})
		}
		if history.confirmClear {
			ui.Column(c.Key("confirm-clear")).FillWidth().Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "清空所有剪贴板历史？").FontSize(13).TextColor(ink)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c.Key("cancel"), "取消").Height(40).AutoFocus().Clicked() {
						history.confirmClear = false
						history.focusSearch = true
					}
					if ui.Button(c.Key("confirm"), "清空历史").Height(40).Clicked() {
						if a.saveClipboard(clipboardState{Recording: history.Recording}) {
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
				history.selected = clipboardEntryKey{}
				reveal = true
			}
			terms := strings.Fields(strings.ToLower(history.query))
			var results []int
			selected := 0
			for index, entry := range history.Entries {
				if len(terms) > 0 {
					text := strings.ToLower(entry.Text)
					if entry.Image != nil {
						text += " 图片"
					}
					if slices.ContainsFunc(terms, func(term string) bool { return !strings.Contains(text, term) }) {
						continue
					}
				}
				if entry.key() == history.selected {
					selected = len(results)
				}
				results = append(results, index)
			}
			if len(results) == 0 {
				empty := "暂无剪贴板历史"
				if len(terms) > 0 {
					empty = "未找到匹配记录"
				}
				ui.Box(c).FillWidth().Grow(1).Center().Children(func() {
					ui.Text(c, empty).FontSize(13).TextColor(muted)
				})
			} else {
				if interactive && c.Shortcut(0, ui.KeyUp) {
					selected, reveal = max(0, selected-1), true
					search.Focus()
				} else if interactive && c.Shortcut(0, ui.KeyDown) {
					selected, reveal = min(len(results)-1, selected+1), true
					search.Focus()
				}
				entry := history.Entries[results[selected]]
				history.selected = entry.key()
				if interactive && history.pendingPaste == nil && (search.Submitted() || c.Shortcut(0, ui.KeyEnter)) {
					history.pendingPaste = &entry
				}
				ui.Scroll(c.Key("entries")).FillWidth().Grow(1).MinHeight(0).Gap(3).Children(func() {
					entries := history.Entries
					for position, index := range results {
						entry := entries[index]
						row := a.clipboardRow(c, index, entry, position == selected, write)
						if position == selected {
							search.ActiveDescendant(row)
							if reveal {
								row.ScrollIntoView()
							}
						}
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
			ui.Text(c, history.problem).FontSize(11).TextColor(ui.Hex("#a65b50")).SingleLine()
		}
	})
}

func (a *island) clipboardRow(c *ui.Context, index int, entry clipboardEntry, selected bool, write func(transfer.Data) error) ui.Element {
	text := strings.TrimSpace(entry.Text)
	count := 0
	for i, r := range text {
		if count == 120 || r == '\r' || r == '\n' {
			text = text[:i] + "…"
			break
		}
		count++
	}
	dimensions := ""
	const rowHeight = 50
	if entry.Image != nil {
		if text == "" {
			text = "图片"
		}
		dimensions = fmt.Sprintf("%d × %d", entry.Image.Width, entry.Image.Height)
	}
	var copy ui.Element
	key := entry.key()
	row := ui.Row(c.Key(key)).FillWidth().Height(rowHeight).Shrink(0).Gap(2).AlignItems(ui.Center).Radius(10).
		HandleInput(func(event ui.InputEvent) bool {
			if event.Kind != ui.InputPointerMove || event.Button != -1 || a.clipboard.selected == key {
				return false
			}
			a.clipboard.selected = key
			return true
		})
	if selected {
		row.Background(accent.Alpha(0.15))
	}
	row.Children(func() {
		if selected {
			ui.Box(c.Key("selection")).Absolute().Left(3).Top(13).Bottom(13).Width(2).
				Radius(1).Background(accent).IgnoreInput(true)
		}
		label := text
		if dimensions != "" {
			label += " " + dimensions
		}
		copy = ui.ButtonBase(c.Key("copy")).Row().Grow(1).MinWidth(0).Height(rowHeight).Padding(0, 9).
			Gap(8).AlignItems(ui.Center).Radius(8).TextColor(muted).Cursor(ui.CursorPointer).Label("复制 " + label)
		copy.Children(func() {
			preview := ui.Box(c.Key("preview")).Size(44, 34).Shrink(0).Radius(6).Clip().Center()
			if entry.Image != nil {
				preview.Background(white.Alpha(0.45))
			}
			preview.Children(func() {
				if entry.Image == nil {
					ui.Icon(c, copyIcon).Size(14, 14)
					return
				}
				if copy.InViewMargin(rowHeight) {
					visual := a.filePreview(shelf.Item{Path: a.clipboardImagePath(entry.Image.ID)})
					if visual != nil && visual.source != nil {
						ui.Image(c, visual.source).Fill().Fit(ui.Contain)
						return
					}
				}
				ui.Icon(c, imageIcon).Size(16, 16).TextColor(muted)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, text).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13).TextColor(ink)
				if dimensions != "" {
					ui.Text(c, dimensions).FontSize(10).TextColor(muted)
				}
			})
			ui.Text(c, entry.CopiedAt.Local().Format("01-02 15:04")).Shrink(0).FontSize(10).TextColor(muted)
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
		if iconButton(c.Key("remove"), trashIcon, "删除此条记录").Size(40, 40).Label("删除记录 " + label).Clicked() {
			next := a.clipboard.clipboardState
			next.Entries = slices.Delete(slices.Clone(next.Entries), index, index+1)
			if a.saveClipboard(next) {
				c.Announce("已删除剪贴板记录")
			}
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
		history.problem = "剪贴板图片读取失败"
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
			if a.expanded || a.hiddenByUser || a.fullscreen {
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
