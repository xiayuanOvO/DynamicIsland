package island

import (
	"crypto/rand"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	todoPreviewMinHeight = 52
	todoPreviewHeader    = 23
	todoPreviewRowHeight = 34
	todoPreviewRows      = 3
	todoPreviewPadding   = 7
	todoListHeight       = 240
	todoEditorGap        = 10
	todoEditorTitle      = 32
	todoEditorName       = 34
	todoEditorRow        = 30
	todoEditorStatus     = 16
	todoEditorHeight     = todoEditorTitle + todoEditorName + 2*todoEditorRow + 3*todoEditorGap
	todoEditorSchedule   = 2*todoEditorRow + 2*todoEditorGap
)

var todoReminderLabels = []string{"不提醒", "准时提醒", "提前 5 分钟", "提前 15 分钟", "提前 30 分钟", "提前 1 小时"}

type todoDraft struct {
	item      todoItem
	scheduled bool
}

func (item todoItem) deadlineLabel(now time.Time) string {
	due := item.Due.Local()
	format := "01-02 15:04"
	if due.Year() != now.Year() {
		format = "2006-01-02 15:04"
	} else if due.YearDay() == now.YearDay() {
		format = "今天 15:04"
	}
	text := due.Format(format)
	if !item.Done && due.Before(now) {
		text = "已逾期 · " + text
	}
	return text
}

func (t *todoComponent) detailHeight() int {
	if t.draft != nil {
		height := todoEditorHeight
		if t.draft.scheduled {
			height += todoEditorSchedule
		}
		if t.problem != "" {
			height += todoEditorStatus + todoEditorGap
		}
		return height
	}
	return todoListHeight
}

func (t *todoComponent) previewHeight() int {
	rows := 0
	for _, item := range t.items {
		if item.Done || rows == todoPreviewRows {
			break
		}
		rows++
	}
	return max(todoPreviewMinHeight, todoPreviewHeader+rows*todoPreviewRowHeight+2*todoPreviewPadding)
}

func (a *island) todoCard(c *ui.Context, progress, opacity float32, frame islandFrame) ui.Element {
	previewHeight := a.todos.previewHeight()
	detailHeight := float32(a.todos.detailHeight())
	if progress == 1 && frame.open == 1 {
		detailHeight = frame.height - panelHeaderHeight - 20
	}
	height := float32(previewHeight) + (detailHeight-float32(previewHeight))*progress
	surface := panelCardSurface(c.Key("todo-surface"), progress, panelBodyWidth/2, height).
		AlignSelf(ui.Start).Label("待办卡片").Opacity(opacity).IgnoreInput(a.clipboard.open || opacity < panelCardInputStart)
	surface.Children(func() {
		if progress < panelCardPreviewEnd {
			a.todoPreview(c, float32(previewHeight), 1-animationPhase(progress, 0, panelCardPreviewEnd))
		}
		if progress > panelCardPreviewEnd {
			ui.Column(c.Key("todo-detail")).Absolute().Top(0).Left(0).Right(0).
				Height(detailHeight).Opacity(animationPhase(progress, panelCardPreviewEnd, panelCardDetailEnd)).
				IgnoreInput(!a.todos.open || progress < panelCardInputStart).Children(func() { a.todoView(c, frame) })
		}
	})
	return surface
}

func (a *island) todoPreview(c *ui.Context, height, opacity float32) {
	card := ui.ButtonBase(c.Key("todo-card")).Column().Absolute().Top(0).Left(0).Right(0).
		Height(height).Radius(13).Cursor(ui.CursorPointer).Label("查看待办事项").
		Opacity(opacity).IgnoreInput(a.todos.open)
	if card.Hovered() {
		card.Background(colorsFor(c.Theme().Dark).glass.Alpha(0.2))
	}
	card.Children(func() {
		ui.Scroll(c.Key("todo-preview-list")).Fill().Padding(todoPreviewPadding, 10).
			Children(func() {
				ui.Row(c.Key("todo-preview-heading")).FillWidth().Height(todoPreviewHeader).Shrink(0).
					Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, todoIcon).Size(14, 14).TextColor(c.Theme().Accent)
					ui.Text(c, "待办事项").SingleLine().FontSize(11).FontWeight(600).TextColor(c.Theme().TextMuted)
				})
				for _, item := range a.todos.items {
					if item.Done {
						break
					}
					ui.Row(c.Key(item.ID)).FillWidth().Height(todoPreviewRowHeight).Shrink(0).
						Gap(7).AlignItems(ui.Center).Children(func() {
						done := item.Done
						if ui.Checkbox(c.Key("done"), &done, "").Label("完成状态 " + item.Title).Changed() {
							item.Done = done
							a.changeTodo(item, false)
						}
						ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
							ui.Text(c, item.Title).SingleLine().Ellipsis("…").FontSize(12).FontWeight(600).TextColor(c.Theme().Text)
							if !item.Due.IsZero() {
								ui.Text(c, item.deadlineLabel(c.Now())).SingleLine().Ellipsis("…").FontSize(9).TextColor(c.Theme().TextMuted)
							}
						})
					})
				}
			})
	})
	if card.Clicked() {
		a.openTodos()
	}
}

func (a *island) openTodos() {
	t := &a.todos
	a.clipboard.open, a.clipboard.confirmClear = false, false
	if t.loadFailed {
		a.loadTodos()
	}
	t.open = true
	a.showPanel()
}

func (t *todoComponent) closeEditor() {
	t.leaving, t.draft = t.draft, nil
}

func (a *island) todoView(c *ui.Context, frame islandFrame) {
	t := &a.todos
	editor := min(1, max(0, frame.editor))
	draft := t.draft
	if draft == nil {
		draft = t.leaving
		if editor == 0 {
			t.leaving = nil
		}
	}
	var footer float32
	if t.problem != "" {
		footer = todoEditorStatus + todoEditorGap
	}
	ui.Themed(c.Key("todos"), panelTheme(c)).Fill().MinHeight(0).Background(ui.Transparent).Children(func() {
		if editor < 1 {
			ui.Column(c.Key("todo-list-page")).Absolute().Top(0).Left(0).Right(0).Bottom(footer).
				Gap(todoEditorGap).Opacity(1-editor).Translate(0, -6*editor).
				IgnoreInput(t.draft != nil).Children(func() { a.todoList(c) })
		}
		if draft != nil && editor > 0 {
			ui.Column(c.Key("todo-editor-page")).Absolute().Top(0).Left(0).Right(0).Bottom(footer).
				Gap(todoEditorGap).Opacity(editor).Translate(0, 6*(1-editor)).
				IgnoreInput(t.draft == nil).Children(func() { a.todoEditor(c, draft, min(1, max(0, frame.schedule))) })
		}
		if t.problem != "" {
			ui.Text(c, t.problem).Absolute().Bottom(0).Left(0).Right(0).Height(todoEditorStatus).
				FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem).SingleLine().Ellipsis("…").Tooltip(t.problem)
		}
	})
}

func (a *island) todoList(c *ui.Context) {
	t := &a.todos
	ui.Row(c).FillWidth().Height(32).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		if iconButton(c.Key("back"), backIcon, "返回岛面板").Clicked() {
			t.open = false
		}
		ui.Text(c, "待办事项").Grow(1).FontSize(18).FontWeight(650).TextColor(c.Theme().Text)
		if ui.Button(c.Key("add"), "新建待办").Height(30).Disabled(t.loadFailed).Clicked() {
			t.draft = &todoDraft{item: todoItem{Due: c.Now().Add(time.Hour).Truncate(time.Minute), LeadMinutes: 15}}
		}
	})
	if len(t.items) == 0 && !t.loadFailed {
		ui.Box(c).FillWidth().Grow(1).Center().Children(func() {
			ui.Text(c, "暂无待办").FontSize(13).TextColor(c.Theme().TextMuted)
		})
	} else {
		if next := t.next(); next != nil && !next.Due.IsZero() {
			c.After(time.Minute - c.Now().Sub(c.Now().Truncate(time.Minute)))
		}
		ui.Scroll(c.Key("todo-list")).FillWidth().Grow(1).MinHeight(0).Children(func() {
			for i, item := range t.items {
				if item.Done && (i == 0 || !t.items[i-1].Done) {
					ui.Text(c, "已完成").Padding(4, 2).FontSize(11).TextColor(c.Theme().TextMuted)
				}
				row := ui.Row(c.Key(item.ID)).FillWidth().Height(50).Shrink(0).Padding(0, 6).
					Gap(8).AlignItems(ui.Center).Radius(7)
				if row.Hovered() {
					row.Background(colorsFor(c.Theme().Dark).glass.Alpha(0.22))
				}
				if i+1 < len(t.items) && item.Done == t.items[i+1].Done {
					row.DrawOver(func(p *ui.Painter, r ui.Rect) {
						p.Fill(ui.Rect{X: r.X + 6, Y: r.Y + r.H - 1, W: r.W - 12, H: 1}, p.Theme().Text.Alpha(0.1), 0)
					})
				}
				row.Children(func() {
					done := item.Done
					if ui.Checkbox(c.Key("done"), &done, "").Label("完成状态 " + item.Title).Changed() {
						updated := item
						updated.Done = done
						a.changeTodo(updated, false)
					}
					edit := ui.ButtonBase(c.Key("edit")).Column().Grow(1).MinWidth(0).Height(46).Justify(ui.Center).AlignItems(ui.Stretch).TextAlign(ui.Start).
						Gap(2).Radius(6).Cursor(ui.CursorPointer).Label("编辑 " + item.Title)
					edit.Children(func() {
						color := c.Theme().Text
						if item.Done {
							color = c.Theme().TextMuted
						}
						ui.Text(c, item.Title).FontSize(13).FontWeight(550).TextColor(color).SingleLine().Ellipsis("…")
						if !item.Due.IsZero() {
							ui.Text(c, item.deadlineLabel(c.Now())).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted)
						}
					})
					if edit.Clicked() {
						draft := todoDraft{item: item, scheduled: !item.Due.IsZero()}
						if !draft.scheduled {
							draft.item.Due = c.Now().Add(time.Hour).Truncate(time.Minute)
							draft.item.LeadMinutes = 15
						}
						t.draft = &draft
					}
				})
			}
		})
	}
}

func (a *island) todoEditor(c *ui.Context, d *todoDraft, schedule float32) {
	existing := d.item.ID != ""
	title := "新建待办"
	if existing {
		title = "编辑待办"
	}
	ui.Text(c, title).Height(todoEditorTitle).Shrink(0).FontSize(18).FontWeight(650).TextColor(c.Theme().Text)
	ui.Column(c.Key("todo-form")).FillWidth().Shrink(0).Children(func() {
		ui.Column(c.Key("todo-fields")).FillWidth().Shrink(0).Gap(todoEditorGap).Children(func() {
			ui.TextInput(c.Key("title"), &d.item.Title).FillWidth().Height(todoEditorName).Shrink(0).Label("待办名称").Placeholder("待办名称").AutoFocus()
			ui.Row(c).FillWidth().Height(todoEditorRow).Shrink(0).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "截止时间").Grow(1).FontSize(13).TextColor(c.Theme().Text)
				ui.Switch(c.Key("scheduled"), &d.scheduled).Label("设置截止时间")
			})
		})
		if schedule > 0 {
			section := ui.Column(c.Key("todo-schedule")).FillWidth().Height(todoEditorSchedule * schedule).Shrink(0).
				Opacity(schedule).IgnoreInput(!d.scheduled || schedule < 1)
			if schedule < 1 {
				section.Clip()
			}
			section.Children(func() {
				ui.Column(c).FillWidth().Shrink(0).Padding(todoEditorGap, 0, 0).Gap(todoEditorGap).Children(func() {
					ui.Row(c).FillWidth().Height(todoEditorRow).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.DateInput(c.Key("date"), &d.item.Due).Grow(1).Label("截止日期")
						ui.TimeInput(c.Key("time"), &d.item.Due).Label("截止时刻")
					})
					ui.Row(c).FillWidth().Height(todoEditorRow).Shrink(0).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "提醒").Grow(1).FontSize(13).TextColor(c.Theme().Text)
						label := todoReminderLabels[slices.Index(todoLeadMinutes, d.item.LeadMinutes)]
						if ui.Select(c.Key("reminder"), &label, todoReminderLabels).Width(148).Height(30).Label("提醒时间").Changed() {
							d.item.LeadMinutes = todoLeadMinutes[slices.Index(todoReminderLabels, label)]
						}
					})
				})
			})
		}
	})
	ui.Row(c.Key("form-actions")).FillWidth().Height(todoEditorRow).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		if existing && iconButton(c.Key("delete"), trashIcon, "删除待办").Clicked() && a.changeTodo(d.item, true) {
			a.todos.closeEditor()
		}
		ui.Box(c).Grow(1)
		if ui.Button(c.Key("cancel"), "取消").Height(todoEditorRow).Clicked() {
			a.todos.closeEditor()
		}
		name := strings.TrimSpace(d.item.Title)
		if ui.PrimaryButton(c.Key("save"), "保存").Height(todoEditorRow).Disabled(name == "").Clicked() {
			item := d.item
			item.Title = name
			if !existing {
				item.ID = rand.Text()
			}
			if !d.scheduled {
				item.Due, item.LeadMinutes = time.Time{}, noTodoReminder
			}
			if a.changeTodo(item, false) {
				a.todos.closeEditor()
			}
		}
	})
}
