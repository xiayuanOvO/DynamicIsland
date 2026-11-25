package island

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"dynamicisland/internal/battery"
	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

func TestWidgetLayoutPackingAndSettingsPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Corrupt/partial panelWidgets JSON should normalize cleanly.
	raw := `{"panelWidgets":[{"id":"media","size":"small"},{"id":"unknown","size":"small"},{"id":"battery","size":"invalid"}]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSettings(path)
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	gotOrder := []panelWidgetID{
		loaded.PanelWidgets[0].ID,
		loaded.PanelWidgets[1].ID,
		loaded.PanelWidgets[2].ID,
		loaded.PanelWidgets[3].ID,
	}
	wantOrder := []panelWidgetID{widgetMedia, widgetBattery, widgetTodo, widgetClipboard}
	if !slices.Equal(gotOrder, wantOrder) {
		t.Fatalf("normalized order = %v, want %v", gotOrder, wantOrder)
	}
	if loaded.PanelWidgets[0].Size != widgetSizeSmall || loaded.PanelWidgets[1].Size != "" {
		t.Fatalf("normalized sizes = %+v", loaded.PanelWidgets)
	}

	// Two adjacent Small widgets pack side-by-side in one row with equal height.
	a := &island{
		settings: appSettings{
			PanelWidgets: panelWidgetSettings{
				{ID: widgetBattery, Size: widgetSizeSmall},
				{ID: widgetMedia, Size: widgetSizeSmall},
				{ID: widgetTodo, Size: widgetSizeLarge},
				{ID: widgetClipboard, Size: widgetSizeMedium},
			},
		},
	}
	a.battery.readings = []battery.Reading{{ID: "mac", Name: "MacBook", Kind: battery.Computer, Percent: 85}}
	a.media.track = nowplaying.Track{Title: "夜曲", Artist: "周杰伦", Duration: 220}
	a.clipboard.Recording = true

	grid := a.panelWidgetLayout()
	if len(grid.slots) != 4 {
		t.Fatalf("expected 4 slots, got %d", len(grid.slots))
	}
	bat, _ := grid.slot(widgetBattery)
	med, _ := grid.slot(widgetMedia)
	todo, _ := grid.slot(widgetTodo)
	clip, _ := grid.slot(widgetClipboard)

	if bat.rect.Y != med.rect.Y || bat.rect.H != med.rect.H {
		t.Fatalf("adjacent small widgets should share row Y and height: bat=%+v med=%+v", bat.rect, med.rect)
	}
	smallW := widgetColSpanPx(2)
	fullW := widgetColSpanPx(4)
	smallH := widgetRowSpanPx(2)
	largeH := widgetRowSpanPx(4)
	if bat.cols != 2 || bat.rows != 2 || med.cols != 2 || med.rows != 2 || todo.cols != 4 || todo.rows != 4 || clip.cols != 4 || clip.rows != 2 {
		t.Fatalf("unexpected grid spans: bat=%dx%d med=%dx%d todo=%dx%d clip=%dx%d",
			bat.cols, bat.rows, med.cols, med.rows, todo.cols, todo.rows, clip.cols, clip.rows)
	}
	if bat.rect.X != 0 || bat.rect.W != smallW || bat.rect.H != smallH ||
		med.rect.X != smallW+widgetGridGap || med.rect.W != smallW || med.rect.H != smallH {
		t.Fatalf("unexpected small widget 2x2 rects: bat=%+v med=%+v", bat.rect, med.rect)
	}
	if todo.rect.W != fullW || todo.rect.H != largeH {
		t.Fatalf("large todo 4x4 rect = %+v, want %.1fx%.1f", todo.rect, fullW, largeH)
	}
	if clip.rect.W != fullW || clip.rect.H != smallH || clip.rect.Y <= todo.rect.Y {
		t.Fatalf("medium clipboard 4x2 rect = %+v, want %.1fx%.1f below todo %+v", clip.rect, fullW, smallH, todo.rect)
	}
}

func TestRightClickWidgetResizesSmallMediumLargeAndPersists(t *testing.T) {
	a := newTestClipboard(t, true, "第一条", "第二条", "第三条", "第四条", "第五条", "第六条")
	a.statePath = filepath.Join(t.TempDir(), "state.json")
	a.battery.readings = []battery.Reading{
		{ID: "mac", Name: "本机", Kind: battery.Computer, Percent: 92, Charging: true},
		{ID: "mouse", Name: "鼠标", Kind: battery.Mouse, Percent: 45},
	}
	a.media.track = nowplaying.Track{Title: "晴天", Artist: "周杰伦", Duration: 269, Previous: true, Toggle: true, Next: true}
	a.todos.items = []todoItem{{ID: "t1", Title: "写周报"}}
	a.expanded = true
	a.shownFrame = a.targetFrame()

	view := ui.NewTester(func(c *ui.Context) {
		a.shownFrame = a.targetFrame()
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
	}, panelWidth, 1200)

	smallW := widgetColSpanPx(2)
	fullW := widgetColSpanPx(4)
	smallH := widgetRowSpanPx(2)
	largeH := widgetRowSpanPx(4)

	// Default medium battery is 4x2 card with device names & percentages.
	batMed, ok := view.Find("设备电量")
	if !ok || math.Abs(float64(batMed.W-fullW)) > 0.5 || math.Abs(float64(batMed.H-smallH)) > 0.5 {
		t.Fatalf("medium battery rect = %+v, want %.1fx%.1f", batMed, fullW, smallH)
	}
	if !view.HasText("本机") || !view.HasText("鼠标") || !view.HasText("92%") || !view.HasText("45%") {
		t.Fatalf("medium battery card should show device names and percentages: %v", view.Texts())
	}

	// Right-click battery -> switch to Small (2x2 ring card).
	if err := view.RightClick("设备电量"); err != nil {
		t.Fatal(err)
	}
	menu := view.Menu()
	if !slices.Equal(menu, []string{"大", "中", "小"}) {
		t.Fatalf("widget context menu = %v, want [大 中 小]", menu)
	}
	if err := view.ChooseMenuItem("小"); err != nil {
		t.Fatal(err)
	}
	batRect, ok := view.Find("设备电量")
	if !ok || math.Abs(float64(batRect.W-smallW)) > 0.5 || math.Abs(float64(batRect.H-smallH)) > 0.5 {
		t.Fatalf("small battery rect = %+v, want %.1fx%.1f", batRect, smallW, smallH)
	}

	// Right-click media -> switch to Small; it should pair side-by-side with small battery!
	if err := view.RightClick("晴天"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("小"); err != nil {
		t.Fatal(err)
	}
	batRect, _ = view.Find("设备电量")
	medRect, ok := view.Find("媒体卡片")
	if !ok || math.Abs(float64(medRect.W-smallW)) > 0.5 || math.Abs(float64(medRect.H-smallH)) > 0.5 || math.Abs(float64(medRect.Y-batRect.Y)) > 0.5 {
		t.Fatalf("small media should sit beside small battery: bat=%+v med=%+v", batRect, medRect)
	}

	// Right-click battery -> switch to Large (4x4 full-width list showing device names).
	if err := view.RightClick("设备电量"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("大"); err != nil {
		t.Fatal(err)
	}
	batRect, _ = view.Find("设备电量")
	if math.Abs(float64(batRect.W-fullW)) > 0.5 || math.Abs(float64(batRect.H-largeH)) > 0.5 {
		t.Fatalf("large battery rect = %+v, want %.1fx%.1f", batRect, fullW, largeH)
	}
	if !view.HasText("鼠标") {
		t.Fatalf("large battery widget should show device name: %v", view.Texts())
	}

	// Right-click clipboard -> switch to Large (4x4, shows >3 entries).
	if err := view.RightClick("剪贴板历史"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("大"); err != nil {
		t.Fatal(err)
	}
	clipRect, ok := view.Find("剪贴板卡片")
	if !ok || math.Abs(float64(clipRect.W-fullW)) > 0.5 || math.Abs(float64(clipRect.H-largeH)) > 0.5 {
		t.Fatalf("large clipboard rect = %+v, want %.1fx%.1f", clipRect, fullW, largeH)
	}
	if !view.HasText("第四条") || !view.HasText("第六条") {
		t.Fatalf("large clipboard widget should show up to 8 entries: %v", view.Texts())
	}

	// Reload settings from disk to verify persistence.
	saved, err := loadSettings(a.settingsPath())
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	reloaded := &island{settings: saved}
	reloaded.clipboard.Recording = true
	if reloaded.effectiveWidgetSize(widgetBattery) != widgetSizeLarge ||
		reloaded.effectiveWidgetSize(widgetMedia) != widgetSizeSmall ||
		reloaded.effectiveWidgetSize(widgetClipboard) != widgetSizeLarge {
		t.Fatalf("persisted sizes mismatch: %+v", saved.PanelWidgets)
	}
}

func TestLongPressDragReordersWidgetsWithoutTriggeringClick(t *testing.T) {
	a := newTestClipboard(t, true, "剪贴板条目")
	a.statePath = filepath.Join(t.TempDir(), "state.json")
	a.todos.items = []todoItem{{ID: "t1", Title: "待办条目"}}
	a.expanded = true
	a.shownFrame = a.targetFrame()

	view := ui.NewTester(func(c *ui.Context) {
		a.shownFrame = a.targetFrame()
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
	}, panelWidth, 400)

	todoBefore, ok1 := view.Find("待办卡片")
	clipBefore, ok2 := view.Find("剪贴板卡片")
	if !ok1 || !ok2 || todoBefore.X >= clipBefore.X {
		t.Fatalf("expected todo on left and clipboard on right: todo=%+v clip=%+v", todoBefore, clipBefore)
	}

	// Long-press clipboard card, drag onto todo card's center, and release.
	startX, startY := clipBefore.X+clipBefore.W/2, clipBefore.Y+clipBefore.H/2
	endX, endY := todoBefore.X+todoBefore.W/2, todoBefore.Y+todoBefore.H/2
	view.Press(startX, startY)
	view.Advance(widgetLongPressDelay + 20*time.Millisecond)
	if !a.widgetDrag.active() {
		t.Fatal("holding for widgetLongPressDelay should enter widget drag mode")
	}
	view.Move(endX, endY)
	view.Release(endX, endY)

	if a.widgetDrag.active() {
		t.Fatal("releasing pointer should end widget drag mode")
	}
	if a.clipboard.open || a.todos.open {
		t.Fatalf("long-press drag should not open clipboard or todo detail views: clip=%v todo=%v", a.clipboard.open, a.todos.open)
	}

	todoAfter, _ := view.Find("待办卡片")
	clipAfter, _ := view.Find("剪贴板卡片")
	if clipAfter.X >= todoAfter.X {
		t.Fatalf("clipboard should now be on the left of todo: clip=%+v todo=%+v", clipAfter, todoAfter)
	}

	// Verify persisted order on disk.
	saved, err := loadSettings(a.settingsPath())
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	clipIdx := slices.IndexFunc(saved.PanelWidgets[:], func(w panelWidgetSetting) bool { return w.ID == widgetClipboard })
	todoIdx := slices.IndexFunc(saved.PanelWidgets[:], func(w panelWidgetSetting) bool { return w.ID == widgetTodo })
	if clipIdx < 0 || todoIdx < 0 || clipIdx > todoIdx {
		t.Fatalf("reordered widget order was not persisted: %+v", saved.PanelWidgets)
	}
}

func TestLongPressDragCancelWithEscapeAndShortClickStillOpens(t *testing.T) {
	a := newTestClipboard(t, true, "剪贴板条目")
	a.battery.readings = []battery.Reading{{ID: "mac", Name: "本机", Kind: battery.Computer, Percent: 80}}
	a.todos.items = []todoItem{{ID: "t1", Title: "待办条目"}}
	a.expanded = true
	a.shownFrame = a.targetFrame()

	view := ui.NewTester(func(c *ui.Context) {
		a.shownFrame = a.targetFrame()
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
		a.shortcuts(c)
	}, panelWidth, 500)

	batBefore, _ := view.Find("设备电量")
	todoBefore, _ := view.Find("待办卡片")

	// Long-press battery, drag down over todo, then press Escape to cancel.
	view.Press(batBefore.X+batBefore.W/2, batBefore.Y+batBefore.H/2)
	view.Advance(widgetLongPressDelay + 20*time.Millisecond)
	view.Move(todoBefore.X+todoBefore.W/2, todoBefore.Y+todoBefore.H/2)
	view.Key(0, ui.KeyEscape)
	view.Release(todoBefore.X+todoBefore.W/2, todoBefore.Y+todoBefore.H/2)

	if a.widgetDrag.active() {
		t.Fatal("Escape should cancel widget drag")
	}
	batAfter, _ := view.Find("设备电量")
	if batAfter.Y != batBefore.Y {
		t.Fatalf("canceling drag with Escape should restore original position: before=%+v after=%+v", batBefore, batAfter)
	}

	// Normal short click on todo should still open the todo panel.
	if err := view.Click("查看待办事项"); err != nil {
		t.Fatal(err)
	}
	if !a.todos.open {
		t.Fatal("normal short click should still open todo panel")
	}
}

func TestEditWidgetsModeHideAddBackAndBottomFooter(t *testing.T) {
	a := newTestClipboard(t, true, "剪贴板条目")
	a.statePath = filepath.Join(t.TempDir(), "state.json")
	a.battery.readings = []battery.Reading{{ID: "mac", Name: "本机", Kind: battery.Computer, Percent: 88}}
	a.todos.items = []todoItem{{ID: "t1", Title: "待办条目"}}
	a.expanded = true
	a.shownFrame = a.targetFrame()

	view := ui.NewTester(func(c *ui.Context) {
		a.shownFrame = a.targetFrame()
		ui.Column(c).Fill().Clip().Children(func() {
			a.header(c, a.shownFrame.open)
			a.body(c, a.shownFrame)
		})
		a.shortcuts(c)
	}, panelWidth, 560)

	// Footer shows "编辑组件" and "设置" horizontally centered below the widgets.
	editBtn, ok1 := view.Find("编辑组件")
	settingsBtn, ok2 := view.Find("设置")
	todoRect, ok3 := view.Find("待办卡片")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("missing footer controls or todo card: edit=%v settings=%v todo=%v", ok1, ok2, ok3)
	}
	if settingsBtn.Y <= todoRect.Y+todoRect.H {
		t.Fatalf("settings button should be below the widget grid: settings=%+v todo=%+v", settingsBtn, todoRect)
	}
	groupCenter := (editBtn.X + settingsBtn.X + settingsBtn.W) / 2
	if math.Abs(float64(groupCenter-panelWidth/2)) > 4 {
		t.Fatalf("footer controls should be horizontally centered: center=%.1f panelMid=%d", groupCenter, panelWidth/2)
	}

	// Not in edit mode: close badges are not shown.
	if _, ok := view.Find("关闭设备电量"); ok {
		t.Fatal("close widget badge should not be visible outside edit mode")
	}

	// Click "编辑组件" -> enters edit mode and shows top-right close badge on widgets.
	if err := view.Click("编辑组件"); err != nil {
		t.Fatal(err)
	}
	if !a.editingWidgets {
		t.Fatal("clicking 编辑组件 should enter edit mode")
	}
	batRect, _ := view.Find("设备电量")
	closeBat, ok := view.Find("关闭设备电量")
	if !ok {
		t.Fatal("close badge for battery should appear in edit mode")
	}
	if closeBat.X < batRect.X+batRect.W/2 || closeBat.Y > batRect.Y+batRect.H/2 {
		t.Fatalf("close badge should be at top-right of battery card: badge=%+v card=%+v", closeBat, batRect)
	}

	// Click close badge -> hides battery widget, shows "+ 设备电量" add-back button, and persists.
	if err := view.Click("关闭设备电量"); err != nil {
		t.Fatal(err)
	}
	if !a.widgetHidden(widgetBattery) {
		t.Fatal("battery widget should be marked hidden after clicking close")
	}
	if _, ok := view.Find("本机"); ok {
		t.Fatal("battery widget content should be hidden after clicking close")
	}
	if _, ok := view.Find("添加设备电量"); !ok {
		t.Fatal("hidden battery widget should appear in the add-back bar")
	}
	saved, err := loadSettings(a.settingsPath())
	if err != nil {
		t.Fatalf("loadSettings: %v", err)
	}
	if !(&island{settings: saved}).widgetHidden(widgetBattery) {
		t.Fatalf("hidden state should persist to settings.json: %+v", saved.PanelWidgets)
	}

	// Click "+ 设备电量" -> restores battery widget.
	if err := view.Click("添加设备电量"); err != nil {
		t.Fatal(err)
	}
	if a.widgetHidden(widgetBattery) {
		t.Fatal("battery widget should no longer be hidden after clicking add-back button")
	}
	if _, ok := view.Find("本机"); !ok {
		t.Fatal("battery widget should be restored after clicking add-back button")
	}
	if _, ok := view.Find("添加设备电量"); ok {
		t.Fatal("add-back button should disappear once battery is restored")
	}

	// Click "完成" -> exits edit mode.
	if err := view.Click("完成"); err != nil {
		t.Fatal(err)
	}
	if a.editingWidgets {
		t.Fatal("clicking 完成 should exit edit mode")
	}
	if _, ok := view.Find("关闭设备电量"); ok {
		t.Fatal("close badges should disappear after exiting edit mode")
	}

	// Escape also exits edit mode without collapsing the island.
	if err := view.Click("编辑组件"); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyEscape)
	if a.editingWidgets || !a.expanded {
		t.Fatalf("Escape should exit edit mode while keeping island expanded: editing=%v expanded=%v", a.editingWidgets, a.expanded)
	}
}
