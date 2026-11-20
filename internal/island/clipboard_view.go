package island

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const clipboardPanelHeight = 280

func (a *island) openClipboard(fromShortcut bool) {
	if !a.clipboard.Recording {
		return
	}
	a.todos.open = false
	a.todos.draft, a.todos.leaving = nil, nil
	a.clipboard.open = true
	a.clipboard.openedByShortcut = fromShortcut
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

func (a *island) clipboardView(c *ui.Context) {
	history := &a.clipboard
	ui.Themed(c.Key("clipboard-history"), todoTheme).FillWidth().Height(clipboardPanelHeight).
		Shrink(0).Gap(8).Background(ui.Transparent).Children(func() {
		ui.Row(c).FillWidth().Height(40).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
			if iconButton(c.Key("back"), backIcon, "返回岛面板").Size(40, 40).Clicked() {
				history.open, history.confirmClear = false, false
			}
			ui.Text(c, "剪贴板历史").Grow(1).FontSize(18).FontWeight(650).TextColor(ink)
			if !history.confirmClear && len(history.Entries) > 0 &&
				ui.Button(c.Key("clear"), "清空历史").Height(40).Clicked() {
				history.confirmClear = true
			}
		})
		if history.confirmClear {
			ui.Column(c.Key("confirm-clear")).FillWidth().Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "清空所有剪贴板历史？").FontSize(13).TextColor(ink)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c.Key("cancel"), "取消").Height(40).AutoFocus().Clicked() {
						history.confirmClear = false
					}
					if ui.Button(c.Key("confirm"), "清空历史").Height(40).Clicked() {
						if a.saveClipboard(clipboardState{Recording: history.Recording}) {
							history.confirmClear = false
							c.Announce("剪贴板历史已清空")
						}
					}
				})
			})
		} else if len(history.Entries) == 0 {
			ui.Box(c).FillWidth().Grow(1).Center().Children(func() {
				ui.Text(c, "暂无剪贴板历史").FontSize(13).TextColor(muted)
			})
		} else {
			ui.Scroll(c.Key("entries")).FillWidth().Grow(1).MinHeight(0).Gap(4).Children(func() {
				for i, entry := range history.Entries {
					a.clipboardRow(c, i, entry)
				}
			})
		}
		if history.problem != "" {
			ui.Text(c, history.problem).FontSize(11).TextColor(ui.Hex("#a65b50")).SingleLine()
		}
	})
}

func (a *island) clipboardRow(c *ui.Context, index int, entry clipboardEntry) {
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
	rowHeight := float32(44)
	if entry.Image != nil {
		rowHeight = 56
		if text == "" {
			text = "图片"
		}
		dimensions = fmt.Sprintf("%d × %d", entry.Image.Width, entry.Image.Height)
	}
	ui.Row(c.Key(entry.key())).FillWidth().Height(rowHeight).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
		label := text
		if dimensions != "" {
			label += " " + dimensions
		}
		copy := ui.ButtonBase(c.Key("copy")).Row().Grow(1).MinWidth(0).Height(rowHeight).Padding(0, 8).
			Gap(8).AlignItems(ui.Center).Radius(8).TextColor(muted).Cursor(ui.CursorPointer).Label("复制 " + label)
		if copy.Hovered() {
			copy.Background(white.Alpha(0.45))
		}
		copy.Children(func() {
			if entry.Image == nil {
				ui.Icon(c, copyIcon).Size(14, 14).Shrink(0)
			} else {
				var visual *filePreview
				if copy.InViewMargin(rowHeight) {
					visual = a.filePreview(shelf.Item{Path: a.clipboardImagePath(entry.Image.ID)})
				}
				ui.Box(c.Key("thumbnail")).Size(52, 40).Shrink(0).Radius(5).Clip().Center().Background(white.Alpha(0.5)).Children(func() {
					if visual != nil && visual.source != nil {
						ui.Image(c, visual.source).Fill().Fit(ui.Contain)
					} else {
						ui.Icon(c, imageIcon).Size(16, 16).TextColor(muted)
					}
				})
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, text).MinWidth(0).SingleLine().Ellipsis("…").FontSize(13).TextColor(ink)
				if dimensions != "" {
					ui.Text(c, dimensions).FontSize(10).TextColor(muted)
				}
			})
			ui.Text(c, entry.CopiedAt.Local().Format("01-02 15:04")).Shrink(0).FontSize(10).TextColor(muted)
		})
		if copy.Clicked() {
			if data, err := a.clipboardData(entry); err != nil {
				a.clipboard.problem = "剪贴板图片读取失败"
			} else if err := mygo.Clipboard.Write(data); err != nil {
				a.clipboard.problem = "复制失败，可重试"
			} else {
				if err := mygo.Clipboard.Flush(); err != nil {
					log.Printf("flush clipboard: %v", err)
				}
				a.clipboard.problem = ""
				c.Toast("已复制")
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
}
