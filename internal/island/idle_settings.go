package island

import "github.com/egoist/mygo/ui"

type idleItemID string

const (
	idleItemFace    idleItemID = "face"
	idleItemWeather idleItemID = "weather"
	idleItemBattery idleItemID = "battery"
	idleItemClock   idleItemID = "clock"
)

var idleItemSpecs = map[idleItemID]struct {
	label string
	width int
}{
	idleItemFace:    {"表情", idleFaceWidth},
	idleItemWeather: {"天气", idleWeatherWidth},
	idleItemBattery: {"设备电量", 0},
	idleItemClock:   {"时间", idleClockWidth},
}

type idleItemSetting struct {
	ID     idleItemID `json:"id"`
	Hidden bool       `json:"hidden,omitempty"`
}

type idleItemSettings [4]idleItemSetting

var defaultIdleItems = idleItemSettings{
	{ID: idleItemFace},
	{ID: idleItemWeather},
	{ID: idleItemBattery},
	{ID: idleItemClock},
}

func (items idleItemSettings) normalized() idleItemSettings {
	if items == (idleItemSettings{}) {
		return items
	}
	var result idleItemSettings
	seen := make(map[idleItemID]bool, len(items))
	index := 0
	for _, item := range items {
		if _, known := idleItemSpecs[item.ID]; known && !seen[item.ID] {
			result[index] = item
			seen[item.ID] = true
			index++
		}
	}
	for _, item := range defaultIdleItems {
		if !seen[item.ID] {
			result[index] = item
			index++
		}
	}
	return result
}

func (s appSettings) idleItems() idleItemSettings {
	if s.IdleItems == (idleItemSettings{}) {
		return defaultIdleItems
	}
	return s.IdleItems
}

func (s appSettings) showsIdleItem(id idleItemID) bool {
	for _, item := range s.idleItems() {
		if item.ID == id {
			return !item.Hidden
		}
	}
	return false
}

func (items *idleItemSettings) move(from, to int) {
	item := items[from]
	if from < to {
		copy(items[from:to], items[from+1:to+1])
	} else {
		copy(items[to+1:from+1], items[to:from])
	}
	items[to] = item
}

var idleReorderIcon = svg(`<path d="M8 5h.1m7.9 0h.1M8 12h.1m7.9 0h.1M8 19h.1m7.9 0h.1" stroke-width="2.5"/>`)

func (a *island) idleDisplaySettings(c *ui.Context) bool {
	items := a.settings.idleItems()
	edited := items
	list := &a.idleSettingsList
	list.KeepOffset = true
	list.Key = func(i int) any { return items[i].ID }
	list.Reorder = func(rows []int, before int) {
		from := rows[0]
		if from < before {
			before--
		}
		edited.move(from, before)
	}
	ui.List(c.Key("idle-display"), list, len(items)).
		FillWidth().Height(float32(len(items) * settingsRowHeight)).Shrink(0).Label("空闲显示顺序").
		Rows(func(row ui.ListRow) {
			c, index := row.Context, row.Index
			item := items[index]
			label := idleItemSpecs[item.ID].label
			settingsRow(c).Gap(8).ContextMenu(func(menu *ui.Menu) {
				if menu.Item("上移").Disabled(index == 0).Chosen() {
					edited.move(index, index-1)
				}
				if menu.Item("下移").Disabled(index == len(items)-1).Chosen() {
					edited.move(index, index+1)
				}
			}).Children(func() {
				ui.Row(c).Grow(1).Height(settingsRowHeight).Gap(8).AlignItems(ui.Center).
					Cursor(ui.CursorGrab).Children(func() {
					ui.Icon(c, idleReorderIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().TextMuted).AccessHidden(true).IgnoreInput(true)
					ui.Text(c, label).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true).IgnoreInput(true)
				})
				shown := !item.Hidden
				toggle := ui.Switch(c.Key("shown"), &shown).Label("显示" + label)
				if toggle.Changed() {
					edited[index].Hidden = !shown
				}
				if toggle.Focused() {
					if index > 0 && c.Shortcut(0, ui.KeyUp) {
						edited.move(index, index-1)
					} else if index < len(items)-1 && c.Shortcut(0, ui.KeyDown) {
						edited.move(index, index+1)
					}
				}
			})
		})
	if edited == items {
		return false
	}
	a.settings.IdleItems = edited
	return true
}
