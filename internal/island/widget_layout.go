package island

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

type panelWidgetID string

const (
	widgetBattery   panelWidgetID = "battery"
	widgetMedia     panelWidgetID = "media"
	widgetTodo      panelWidgetID = "todo"
	widgetClipboard panelWidgetID = "clipboard"
)

type widgetSize string

const (
	widgetSizeSmall  widgetSize = "small"
	widgetSizeMedium widgetSize = "medium"
	widgetSizeLarge  widgetSize = "large"
)

const (
	widgetGridCols            = 4
	widgetGridGap             = overviewCardGap
	widgetGridCell            = float32(panelBodyWidth-(widgetGridCols-1)*widgetGridGap) / widgetGridCols
	widgetLongPressDelay      = 350 * time.Millisecond
	widgetLongPressMoveLimit  = 6
	clipboardLargePreviewRows = 10
)

func widgetGridSpan(size widgetSize) (cols, rows int) {
	switch size {
	case widgetSizeSmall:
		return 2, 2
	case widgetSizeLarge:
		return 4, 4
	default:
		return 4, 2
	}
}

func widgetSpanPx(cells int) float32 {
	if cells <= 0 {
		return 0
	}
	return float32(cells)*widgetGridCell + float32(cells-1)*widgetGridGap
}

func widgetCellOffsetPx(index int) float32 {
	return float32(index) * (widgetGridCell + widgetGridGap)
}

type panelWidgetSetting struct {
	ID   panelWidgetID `json:"id"`
	Size widgetSize    `json:"size,omitempty"`
}

type panelWidgetSettings [4]panelWidgetSetting

var defaultPanelWidgets = panelWidgetSettings{
	{ID: widgetBattery},
	{ID: widgetMedia},
	{ID: widgetTodo},
	{ID: widgetClipboard},
}

func validPanelWidgetID(id panelWidgetID) bool {
	switch id {
	case widgetBattery, widgetMedia, widgetTodo, widgetClipboard:
		return true
	default:
		return false
	}
}

func validWidgetSize(size widgetSize) bool {
	switch size {
	case "", widgetSizeSmall, widgetSizeMedium, widgetSizeLarge:
		return true
	default:
		return false
	}
}

func (widgets panelWidgetSettings) normalized() panelWidgetSettings {
	if widgets == (panelWidgetSettings{}) {
		return widgets
	}
	var result panelWidgetSettings
	seen := make(map[panelWidgetID]bool, len(widgets))
	index := 0
	for _, item := range widgets {
		if validPanelWidgetID(item.ID) && !seen[item.ID] {
			if !validWidgetSize(item.Size) {
				item.Size = ""
			}
			result[index] = item
			seen[item.ID] = true
			index++
		}
	}
	for _, item := range defaultPanelWidgets {
		if !seen[item.ID] {
			result[index] = item
			index++
		}
	}
	return result
}

func (s appSettings) panelWidgets() panelWidgetSettings {
	if s.PanelWidgets == (panelWidgetSettings{}) {
		return defaultPanelWidgets
	}
	return s.PanelWidgets.normalized()
}

type panelWidgetSlot struct {
	id         panelWidgetID
	size       widgetSize
	col, row   int
	cols, rows int
	rect       ui.Rect
}

type panelWidgetGrid struct {
	slots  []panelWidgetSlot
	height float32
}

func (g panelWidgetGrid) slot(id panelWidgetID) (panelWidgetSlot, bool) {
	for _, s := range g.slots {
		if s.id == id {
			return s, true
		}
	}
	return panelWidgetSlot{}, false
}

func (a *island) widgetVisible(id panelWidgetID) bool {
	switch id {
	case widgetBattery:
		return len(a.battery.readings) > 0
	case widgetMedia:
		return a.media.track.Title != ""
	case widgetTodo:
		return true
	case widgetClipboard:
		return a.clipboard.Recording
	default:
		return false
	}
}

func (a *island) effectiveWidgetSize(id panelWidgetID) widgetSize {
	for _, item := range a.settings.panelWidgets() {
		if item.ID == id && item.Size != "" {
			return item.Size
		}
	}
	switch id {
	case widgetBattery, widgetMedia:
		return widgetSizeMedium
	case widgetTodo:
		if a.clipboard.Recording {
			return widgetSizeSmall
		}
		return widgetSizeMedium
	case widgetClipboard:
		return widgetSizeSmall
	default:
		return widgetSizeMedium
	}
}

func (a *island) panelWidgetLayout() panelWidgetGrid {
	widgets := a.settings.panelWidgets()
	order := make([]panelWidgetID, 0, len(widgets))
	for _, item := range widgets {
		if a.widgetVisible(item.ID) {
			order = append(order, item.ID)
		}
	}
	return a.layoutWidgets(order)
}

func (a *island) layoutWidgets(order []panelWidgetID) panelWidgetGrid {
	slots := make([]panelWidgetSlot, 0, len(order))
	var occupied [][widgetGridCols]bool
	canPlace := func(row, col, cols, rows int) bool {
		if col < 0 || col+cols > widgetGridCols {
			return false
		}
		for r := row; r < row+rows; r++ {
			if r >= len(occupied) {
				continue
			}
			for c := col; c < col+cols; c++ {
				if occupied[r][c] {
					return false
				}
			}
		}
		return true
	}
	mark := func(row, col, cols, rows int) {
		for len(occupied) < row+rows {
			occupied = append(occupied, [widgetGridCols]bool{})
		}
		for r := row; r < row+rows; r++ {
			for c := col; c < col+cols; c++ {
				occupied[r][c] = true
			}
		}
	}

	maxRow := 0
	for _, id := range order {
		size := a.effectiveWidgetSize(id)
		cols, rows := widgetGridSpan(size)
		placedRow, placedCol := 0, 0
	findSlot:
		for row := 0; ; row++ {
			for col := 0; col <= widgetGridCols-cols; col++ {
				if canPlace(row, col, cols, rows) {
					placedRow, placedCol = row, col
					break findSlot
				}
			}
		}
		mark(placedRow, placedCol, cols, rows)
		if endRow := placedRow + rows; endRow > maxRow {
			maxRow = endRow
		}
		slots = append(slots, panelWidgetSlot{
			id:   id,
			size: size,
			col:  placedCol,
			row:  placedRow,
			cols: cols,
			rows: rows,
			rect: ui.Rect{
				X: widgetCellOffsetPx(placedCol),
				Y: widgetCellOffsetPx(placedRow),
				W: widgetSpanPx(cols),
				H: widgetSpanPx(rows),
			},
		})
	}
	return panelWidgetGrid{slots: slots, height: widgetSpanPx(maxRow)}
}

func (a *island) setWidgetSize(id panelWidgetID, size widgetSize) {
	if !validPanelWidgetID(id) || !validWidgetSize(size) {
		return
	}
	widgets := a.settings.panelWidgets()
	changed := false
	for i := range widgets {
		if widgets[i].ID == id {
			if widgets[i].Size != size {
				widgets[i].Size = size
				changed = true
			}
			break
		}
	}
	if !changed {
		return
	}
	a.settings.PanelWidgets = widgets
	a.persistWidgetSettings()
	now := time.Now()
	a.leaveSince = now.Add(800 * time.Millisecond)
	a.animation.retarget(a.targetFrame(), now)
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *island) reorderVisibleWidgets(visibleOrder []panelWidgetID) {
	widgets := a.settings.panelWidgets()
	byID := make(map[panelWidgetID]panelWidgetSetting, len(widgets))
	for _, w := range widgets {
		byID[w.ID] = w
	}
	var updated panelWidgetSettings
	visIdx := 0
	for i, w := range widgets {
		if slices.Contains(visibleOrder, w.ID) {
			updated[i] = byID[visibleOrder[visIdx]]
			visIdx++
		} else {
			updated[i] = w
		}
	}
	if updated == widgets {
		return
	}
	a.settings.PanelWidgets = updated
}

func (a *island) persistWidgetSettings() {
	if a.statePath == "" {
		return
	}
	_ = a.settings.save(a.settingsPath())
}

type widgetDragState struct {
	pressing      bool
	dragging      bool
	suppressClick bool
	id            panelWidgetID
	pressAt       time.Time
	startX        float32
	startY        float32
	pointerX      float32
	pointerY      float32
	originRect    ui.Rect
	originWidgets panelWidgetSettings
}

func (d *widgetDragState) active() bool {
	return d.dragging
}

func (d *widgetDragState) cancel(a *island) {
	if d.dragging {
		a.settings.PanelWidgets = d.originWidgets
	}
	*d = widgetDragState{}
}

func (d *widgetDragState) beginPress(a *island, grid panelWidgetGrid, x, y float32, now time.Time) {
	if a.todos.open || a.clipboard.open || a.media.scrubbing {
		return
	}
	for _, slot := range grid.slots {
		if x >= slot.rect.X && x <= slot.rect.X+slot.rect.W && y >= slot.rect.Y && y <= slot.rect.Y+slot.rect.H {
			*d = widgetDragState{
				pressing:      true,
				id:            slot.id,
				pressAt:       now,
				startX:        x,
				startY:        y,
				pointerX:      x,
				pointerY:      y,
				originRect:    slot.rect,
				originWidgets: a.settings.PanelWidgets,
			}
			return
		}
	}
}

func (d *widgetDragState) updatePointer(a *island, x, y float32, now time.Time) time.Duration {
	if !d.pressing && !d.dragging {
		return 0
	}
	d.pointerX, d.pointerY = x, y
	if a.todos.open || a.clipboard.open || a.media.scrubbing {
		d.cancel(a)
		return 0
	}
	if !d.dragging {
		dx, dy := x-d.startX, y-d.startY
		if dx*dx+dy*dy > widgetLongPressMoveLimit*widgetLongPressMoveLimit {
			*d = widgetDragState{}
			return 0
		}
		elapsed := now.Sub(d.pressAt)
		if elapsed < widgetLongPressDelay {
			return widgetLongPressDelay - elapsed
		}
		d.dragging = true
	}
	d.updateDropOrder(a)
	return 0
}

func (d *widgetDragState) updateDropOrder(a *island) {
	widgets := a.settings.panelWidgets()
	visible := make([]panelWidgetID, 0, len(widgets))
	curIdx := -1
	for _, item := range widgets {
		if a.widgetVisible(item.ID) {
			if item.ID == d.id {
				curIdx = len(visible)
			}
			visible = append(visible, item.ID)
		}
	}
	if curIdx < 0 || len(visible) <= 1 {
		return
	}
	others := make([]panelWidgetID, 0, len(visible)-1)
	others = append(others, visible[:curIdx]...)
	others = append(others, visible[curIdx+1:]...)

	bestIdx := curIdx
	bestDist := float32(-1)
	cand := make([]panelWidgetID, len(visible))
	for i := range len(visible) {
		copy(cand[:i], others[:i])
		cand[i] = d.id
		copy(cand[i+1:], others[i:])
		layout := a.layoutWidgets(cand)
		slot, ok := layout.slot(d.id)
		if !ok {
			continue
		}
		cx := slot.rect.X + slot.rect.W/2
		cy := slot.rect.Y + slot.rect.H/2
		dx := d.pointerX - cx
		dy := d.pointerY - cy
		dist := dx*dx + dy*dy
		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	if bestIdx == curIdx {
		return
	}
	copy(cand[:bestIdx], others[:bestIdx])
	cand[bestIdx] = d.id
	copy(cand[bestIdx+1:], others[bestIdx:])
	a.reorderVisibleWidgets(cand)
}

func (d *widgetDragState) endPress(a *island, canceled bool) {
	if !d.pressing && !d.dragging {
		return
	}
	wasDragging := d.dragging
	if wasDragging {
		if canceled {
			a.settings.PanelWidgets = d.originWidgets
		} else {
			a.persistWidgetSettings()
		}
	}
	*d = widgetDragState{suppressClick: wasDragging}
}

func (a *island) widgetSizeMenu(m *ui.Menu, id panelWidgetID) {
	current := a.effectiveWidgetSize(id)
	for _, option := range []struct {
		label string
		size  widgetSize
	}{
		{"大", widgetSizeLarge},
		{"中", widgetSizeMedium},
		{"小", widgetSizeSmall},
	} {
		if m.Item(option.label).Radio(current == option.size).Chosen() {
			a.setWidgetSize(id, option.size)
		}
	}
}
