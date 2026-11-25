package island

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestPanelCardsFollowWindowHeight(t *testing.T) {
	for _, test := range []struct {
		name, label, button string
		right               bool
		withClipboard       bool
	}{
		{"todos-fullwidth", "待办卡片", "查看待办事项", false, false},
		{"todos-split", "待办卡片", "查看待办事项", false, true},
		{"clipboard", "剪贴板卡片", "查看剪贴板历史", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &island{}
			if test.withClipboard {
				a = newTestClipboard(t, true, "示例记录")
			}
			if !test.right {
				a.todos.items = []todoItem{{ID: "example", Title: "示例待办"}}
			}
			a.expanded = true
			overview := a.targetFrame()
			a.shownFrame = overview
			a.animation = islandAnimation{from: overview, to: overview, started: time.Now()}
			view := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Clip().Children(func() {
					ui.Box(c).Height(panelHeaderHeight).Shrink(0)
					a.body(c, a.shownFrame)
				})
			}, panelWidth, int(overview.height))

			now := a.animation.started
			initial, ok := view.Find(test.label)
			if !ok {
				t.Fatal("card is missing")
			}
			widthAnimates := initial.W < panelBodyWidth
			before := initial
			for _, button := range []string{test.button, "返回岛面板"} {
				if err := view.Click(button); err != nil {
					t.Fatal(err)
				}
				opening := button == test.button
				if a.animation.started.After(now) {
					now = a.animation.started
				}
				a.animation.retarget(a.targetFrame(), now)
				for tick := range int(islandResizeDuration/windowFrameInterval) + 1 {
					elapsed := windowFrameInterval
					if tick == 4 {
						elapsed *= 4 // Simulate a missed display refresh.
					}
					now = now.Add(elapsed)
					a.shownFrame = a.animation.advance(now).aligned()
					view.Advance(elapsed)
					view.SetSize(panelWidth, int(a.shownFrame.height))
					card, ok := view.Find(test.label)
					if !ok {
						t.Fatal("card disappeared during the transition")
					}
					if delta := card.Y + card.H + 20 - a.shownFrame.height; math.Abs(float64(delta)) > 1 {
						t.Fatalf("%s: card and window heights differ by %.1f points", button, delta)
					}
					edge := card.X - initial.X
					if test.right {
						edge += card.W - initial.W
					}
					if math.Abs(float64(edge)) > 1 {
						t.Fatalf("%s: the card moved away from its original edge by %.1f points", button, edge)
					}
					if opening && card.W < before.W || !opening && card.W > before.W {
						t.Fatalf("%s: card width moved in the wrong direction from %.1f to %.1f", button, before.W, card.W)
					}
					if tick == 0 && (a.shownFrame.height == a.targetFrame().height || (widthAnimates && card.W == before.W)) {
						t.Fatalf("%s: the first frame did not show an in-progress transition", button)
					}
					before = card
				}
				if a.shownFrame != a.targetFrame() {
					t.Fatalf("%s: the window did not settle at the target frame", button)
				}
				if opening && test.right && !view.Focused("搜索剪贴板历史") {
					t.Fatal("the expanded clipboard panel should focus search")
				}
			}
		})
	}
}

func TestClipboardOpeningFadesOverviewBeforeRemovingIt(t *testing.T) {
	a := newTestClipboard(t, true, "示例记录")
	a.media.track = nowplaying.Track{Title: "示例曲目"}
	a.todos.items = []todoItem{{ID: "example", Title: "示例待办"}}
	a.expanded = true
	overview := a.targetFrame()
	a.shownFrame = overview
	now := time.Now()
	a.animation = islandAnimation{from: overview, to: overview, started: now}
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
	}, panelWidth, int(overview.height))
	overviewLabels := []string{"示例曲目", "待办卡片", "示例待办"}
	initial := make([]ui.Rect, len(overviewLabels))
	for i, label := range overviewLabels {
		var ok bool
		initial[i], ok = view.Find(label)
		if !ok || initial[i].H == 0 {
			t.Fatalf("%q should be fully visible in the overview", label)
		}
	}
	if err := view.Click("查看剪贴板历史"); err != nil {
		t.Fatal(err)
	}
	for _, elapsed := range []time.Duration{35 * time.Millisecond, 60 * time.Millisecond, 75 * time.Millisecond, 100 * time.Millisecond} {
		frame, _ := a.animation.sample(a.animation.started.Add(elapsed))
		a.shownFrame = frame.aligned()
		view.SetSize(panelWidth, int(a.shownFrame.height))
		fading := frame.clipboard < panelCardPreviewEnd
		for i, label := range overviewLabels {
			box, ok := view.Find(label)
			if fading && (!ok || math.Abs(float64(box.W-initial[i].W)) > 0.5 || math.Abs(float64(box.H-initial[i].H)) > 0.5) {
				t.Fatalf("%v: %q should stay whole while fading; changed from %+v to %+v", elapsed, label, initial[i], box)
			}
			if fading && label == "待办卡片" && math.Abs(float64(box.Y-initial[i].Y)) > 0.5 {
				t.Fatalf("%v: the fading todo card moved from y=%.1f to y=%.1f", elapsed, initial[i].Y, box.Y)
			}
			if !fading && ok {
				t.Fatalf("%v: %q should be gone when the history appears", elapsed, label)
			}
		}
	}
}

func TestPanelOpeningKeepsMediaCardGeometry(t *testing.T) {
	for _, style := range []int{mediaCardDefault, mediaCardLargeCover} {
		a := newTestClipboard(t, true, "示例记录")
		a.settings.MediaCardStyle = style
		a.media.track = nowplaying.Track{Title: "示例曲目", Artist: "示例艺人", Duration: 190, Previous: true, Toggle: true}
		a.expanded = true
		a.shownFrame = a.targetFrame()
		now := time.Now()
		a.animation = islandAnimation{from: a.shownFrame, to: a.shownFrame, started: now}
		view := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Clip().Children(func() {
				ui.Box(c).Height(panelHeaderHeight).Shrink(0)
				a.body(c, a.shownFrame)
			})
		}, panelWidth, int(a.shownFrame.height))
		labels := []string{"示例曲目", "示例艺人", "上一首", "3:10"}
		bounds := make([]ui.Rect, len(labels))
		for i, label := range labels {
			box, ok := view.Find(label)
			if !ok || box.H <= 0 {
				t.Fatalf("style %d: could not find %q before opening", style, label)
			}
			bounds[i] = box
		}
		if err := view.Click("查看剪贴板历史"); err != nil {
			t.Fatal(err)
		}
		for _, elapsed := range []time.Duration{35 * time.Millisecond, 60 * time.Millisecond, 75 * time.Millisecond} {
			frame, _ := a.animation.sample(a.animation.started.Add(elapsed))
			a.shownFrame = frame.aligned()
			view.SetSize(panelWidth, int(a.shownFrame.height))
			for i, label := range labels {
				if box, ok := view.Find(label); !ok || box != bounds[i] {
					t.Fatalf("style %d at %v: %q changed from %+v to %+v", style, elapsed, label, bounds[i], box)
				}
			}
		}
		frame, _ := a.animation.sample(a.animation.started.Add(100 * time.Millisecond))
		a.shownFrame = frame.aligned()
		view.SetSize(panelWidth, int(a.shownFrame.height))
		if !view.HasText("示例记录") || view.HasText("示例曲目") {
			t.Fatalf("style %d: the music card should finish fading out as the history appears", style)
		}
	}
}

func TestTodoRepeatReschedulesOnCompletionAndFormatsLabel(t *testing.T) {
	// Friday -> next weekday should jump to Monday.
	fri := time.Date(2026, time.November, 20, 18, 0, 0, 0, time.Local)
	nextMon := nextTodoDue(fri, todoRepeatWeekdays, fri)
	if nextMon.Weekday() != time.Monday || nextMon.Day() != 23 || nextMon.Hour() != 18 {
		t.Fatalf("weekdays repeat from Friday should land on Monday 18:00, got %v", nextMon)
	}
	nextDay := nextTodoDue(fri, todoRepeatDaily, fri)
	if nextDay.Day() != 21 {
		t.Fatalf("daily repeat should add 1 day, got %v", nextDay)
	}
	nextWeek := nextTodoDue(fri, todoRepeatWeekly, fri)
	if nextWeek.Day() != 27 {
		t.Fatalf("weekly repeat should add 7 days, got %v", nextWeek)
	}

	a := island{statePath: filepath.Join(t.TempDir(), "settings.json")}
	due := time.Now().Add(time.Hour).Truncate(time.Minute)
	item := todoItem{ID: "drink", Title: "喝水", Due: due, LeadMinutes: 15, Repeat: todoRepeatDaily, Notified: true}
	if !a.changeTodo(item, false) {
		t.Fatal("failed to save repeating todo")
	}
	if label := a.todos.items[0].deadlineLabel(time.Now()); !strings.Contains(label, "每天") {
		t.Fatalf("repeating todo deadline label should include repeat text, got %q", label)
	}

	// Marking a repeating todo done should reschedule it, keep it undone, and reset Notified.
	completed := a.todos.items[0]
	completed.Done = true
	if !a.changeTodo(completed, false) {
		t.Fatal("failed to complete repeating todo")
	}
	got := a.todos.items[0]
	if got.Done || got.Notified || !got.Due.After(due) {
		t.Fatalf("completing repeating todo should reschedule and re-arm reminder: %+v", got)
	}

	reloaded := island{statePath: a.statePath}
	reloaded.loadTodos()
	if reloaded.todos.loadFailed || len(reloaded.todos.items) != 1 || reloaded.todos.items[0].Repeat != todoRepeatDaily {
		t.Fatalf("repeat setting should survive reload: %+v", reloaded.todos.items)
	}
}

func TestOverviewAdaptiveWidthEqualHeightAndClipboardPreviewCopy(t *testing.T) {
	// 1. Clipboard disabled: todo card spans full panelBodyWidth.
	a := &island{}
	a.todos.items = []todoItem{{ID: "1", Title: "整理文档"}}
	a.expanded = true
	frame := a.targetFrame()
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			a.header(c, frame.open)
			a.body(c, frame)
		})
	}, panelWidth, int(frame.height))
	todo, ok := view.Find("待办卡片")
	if !ok || math.Abs(float64(todo.W-panelBodyWidth)) > 0.5 {
		t.Fatalf("todo card should span full panelBodyWidth (%d) when clipboard is disabled, got %+v", panelBodyWidth, todo)
	}
	if _, hasSettings := view.Find("设置"); !hasSettings {
		t.Fatal("expanded header should show the settings icon button")
	}

	// Collapsed header should hide the settings icon button.
	collapsed := ui.NewTester(func(c *ui.Context) {
		a.expanded = false
		a.header(c, 0)
	}, compactWidth, compactHeight)
	if _, hasSettings := collapsed.Find("设置"); hasSettings {
		t.Fatal("collapsed header should not show the settings icon button")
	}

	// 2. Clipboard enabled with 3 items and 1 todo: both cards match height and show recent clipboard entries.
	b := newTestClipboard(t, true, "最新一条", "第二条记录", "第三条记录", "第四条不预览")
	b.todos.items = []todoItem{{ID: "1", Title: "唯一待办"}}
	b.expanded = true
	bFrame := b.targetFrame()
	var copied string
	bView := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			toolsPreview := float32(b.toolsPreviewHeight())
			b.todoCard(c, 0, 1, bFrame)
			b.clipboardPreviewCard(c, toolsPreview, 1, func(data transfer.Data) error {
				text, err := data.Read(transfer.Text)
				copied = string(text)
				return err
			})
		})
	}, panelWidth, int(bFrame.height))
	if !bView.HasText("最新一条") || !bView.HasText("第二条记录") || !bView.HasText("第三条记录") || bView.HasText("第四条不预览") {
		t.Fatalf("clipboard preview should show latest 3 entries: %v", bView.Texts())
	}
	if err := bView.Click("复制 第二条记录"); err != nil {
		t.Fatal(err)
	}
	if copied != "第二条记录" || b.clipboard.open {
		t.Fatalf("clicking a preview row should copy directly without opening full panel: copied=%q open=%v", copied, b.clipboard.open)
	}

	// Verify equal height in full body render.
	fullView := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			b.body(c, bFrame)
		})
	}, panelWidth, int(bFrame.height))
	todoRect, ok1 := fullView.Find("待办卡片")
	clipRect, ok2 := fullView.Find("剪贴板卡片")
	if !ok1 || !ok2 || math.Abs(float64(todoRect.H-clipRect.H)) > 0.5 || math.Abs(float64(todoRect.W-clipRect.W)) > 0.5 {
		t.Fatalf("todo and clipboard cards should have equal width and height: todo=%+v clip=%+v", todoRect, clipRect)
	}
}

func TestTodoRepeatDebounceClearCompletedOverdueAndQuickAdd(t *testing.T) {
	a := &island{statePath: filepath.Join(t.TempDir(), "settings.json"), expanded: true}
	now := time.Now().Truncate(time.Minute)
	due := now.Add(time.Hour)
	overdue := now.Add(-time.Hour)

	if !a.changeTodo(todoItem{ID: "rep", Title: "每日打卡", Due: due, LeadMinutes: 15, Repeat: todoRepeatDaily}, false) {
		t.Fatal("failed to add repeating todo")
	}
	if !a.changeTodo(todoItem{ID: "late", Title: "提交报告", Due: overdue, LeadMinutes: noTodoReminder}, false) {
		t.Fatal("failed to add overdue todo")
	}

	// 1. Overdue detection: only uncompleted past-due items are overdue.
	if !a.todos.items[0].overdue(now) {
		t.Fatalf("expected %+v to be overdue", a.todos.items[0])
	}
	if (todoItem{Due: overdue, Done: true}).overdue(now) || (todoItem{Due: due}).overdue(now) {
		t.Fatal("completed or future items must not be overdue")
	}

	// 2. Quick '+' button in todo preview opens the editor directly.
	frame := a.targetFrame()
	preview := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, frame)
		})
	}, panelWidth, int(frame.height))
	if err := preview.Click("新建待办"); err != nil {
		t.Fatal(err)
	}
	if !a.todos.open || a.todos.draft == nil {
		t.Fatal("clicking quick-add '+' should open the todo editor directly")
	}

	// 3. Pressing Enter in editor title input ignores empty title and saves non-empty title.
	editorView := ui.NewTester(func(c *ui.Context) {
		a.todoView(c, a.targetFrame())
	}, panelWidth, todoEditorHeight+60)
	editorView.Key(0, ui.KeyEnter)
	if a.todos.draft == nil {
		t.Fatal("Enter with empty title should not save or close editor")
	}
	editorView.Type("买牛奶")
	editorView.Key(0, ui.KeyEnter)
	if a.todos.draft != nil {
		t.Fatal("Enter with non-empty title should save and close editor")
	}
	foundMilk := false
	for _, item := range a.todos.items {
		if item.Title == "买牛奶" {
			foundMilk = true
		}
	}
	if !foundMilk {
		t.Fatalf("expected '买牛奶' to be saved via Enter, got %+v", a.todos.items)
	}

	// 4. Repeating todo checkbox shows next-due toast and debounces rapid clicks within 400ms.
	listView := ui.NewTester(func(c *ui.Context) {
		a.todoList(c)
	}, panelWidth, todoListHeight)
	if _, hasClear := listView.Find("清空已完成"); hasClear {
		t.Fatal("clear completed button should not appear when no completed items exist")
	}
	if err := listView.Click("完成状态 每日打卡"); err != nil {
		t.Fatal(err)
	}
	var firstNext time.Time
	for _, item := range a.todos.items {
		if item.ID == "rep" {
			firstNext = item.Due
		}
	}
	if !firstNext.After(due) {
		t.Fatalf("repeating todo should have advanced past %v, got %v", due, firstNext)
	}
	// Rapid click within 400ms must be ignored.
	listView.Advance(100 * time.Millisecond)
	if err := listView.Click("完成状态 每日打卡"); err != nil {
		t.Fatal(err)
	}
	for _, item := range a.todos.items {
		if item.ID == "rep" && !item.Due.Equal(firstNext) {
			t.Fatalf("rapid click within 400ms should not advance due again: got %v, want %v", item.Due, firstNext)
		}
	}
	// After 400ms, another completion advances again.
	listView.Advance(todoRepeatDebounce)
	if err := listView.Click("完成状态 每日打卡"); err != nil {
		t.Fatal(err)
	}
	for _, item := range a.todos.items {
		if item.ID == "rep" && !item.Due.After(firstNext) {
			t.Fatalf("click after debounce window should advance due: got %v, prev %v", item.Due, firstNext)
		}
	}

	// 5. Completing a non-repeating todo shows '清空已完成' and clicking it clears completed items.
	if err := listView.Click("完成状态 提交报告"); err != nil {
		t.Fatal(err)
	}
	if _, hasClear := listView.Find("清空已完成"); !hasClear {
		t.Fatal("clear completed button should appear once a completed item exists")
	}
	if err := listView.Click("清空已完成"); err != nil {
		t.Fatal(err)
	}
	if _, hasClear := listView.Find("清空已完成"); hasClear {
		t.Fatal("clear completed button should disappear after clearing")
	}
	for _, item := range a.todos.items {
		if item.Done {
			t.Fatalf("expected all completed items to be removed, found %+v", item)
		}
	}
	reloaded := island{statePath: a.statePath}
	reloaded.loadTodos()
	for _, item := range reloaded.todos.items {
		if item.Done {
			t.Fatalf("cleared completed items should not return after reload: %+v", item)
		}
	}
}
