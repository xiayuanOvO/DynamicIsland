package island

import (
	"context"
	"fmt"
	"math"
	"time"

	"dynamicisland/internal/battery"
	"github.com/egoist/mygo/ui"
)

const (
	batteryRowHeight = 50
	batteryRowGap    = 10
)

type islandBattery struct {
	readings  []battery.Reading
	refreshAt time.Time
	reading   bool
}

var (
	batteryIcon     = svg(`<rect x="4" y="7" width="15" height="10" rx="2"/><path d="M22 10v4M7 10h5v4H7z"/>`)
	computerIcon    = svg(`<rect x="5" y="5" width="14" height="11" rx="1"/><path d="m5 16-2 3h18l-2-3"/>`)
	mouseIcon       = svg(`<rect x="6" y="3" width="12" height="18" rx="6"/><path d="M12 6v4"/>`)
	keyboardIcon    = svg(`<rect x="2" y="6" width="20" height="12" rx="2"/><path d="M6 10h.1m4 0h.1m4 0h.1m4 0h.1M6 14h.1m4 0h8"/>`)
	trackpadIcon    = svg(`<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M9 16h6"/>`)
	headphonesIcon  = svg(`<path d="M4 14v-2a8 8 0 0 1 16 0v2"/><rect x="4" y="12" width="4" height="7" rx="2"/><rect x="16" y="12" width="4" height="7" rx="2"/>`)
	earbudIcon      = svg(`<path d="M15 10v9a2 2 0 0 1-4 0v-6H9a4 4 0 0 1 0-8h3a3 3 0 0 1 3 3v2Z"/><path d="M8 8v2"/>`)
	batteryCaseIcon = svg(`<rect x="4" y="6" width="16" height="12" rx="4"/><path d="M5 11h14m-7 3h.1"/>`)
	chargingIcon    = svg(`<path d="m14 3-8 10h5l-1 8 8-10h-5Z" fill="currentColor" stroke-width="1"/>`)
	chargeGreen     = ui.Hex("#237448")
	batteryLow      = ui.Hex("#a63338")
)

func (a *island) updateBattery(c *ui.Context) {
	b := &a.battery
	showOverview := a.expanded && !a.todos.open && !a.clipboard.open
	showIdle := a.showsIdleCapsule() && a.settings.showsIdleItem(idleItemBattery)
	if !battery.Supported || (!showOverview && !showIdle) || !a.win.IsVisible() || b.reading {
		return
	}
	if delay := b.refreshAt.Sub(c.Now()); delay > 0 {
		c.After(delay)
		return
	}
	b.reading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		readings := battery.Read(ctx)
		a.win.Update(func() {
			b.reading = false
			b.readings = readings
			b.refreshAt = time.Now().Add(30 * time.Second)
		})
	}()
}

func (a *island) overviewLeadHeight() int {
	height := 0
	if len(a.battery.readings) > 0 {
		height += batteryRowHeight + batteryRowGap
	}
	if a.media.track.Title != "" {
		height += a.settings.cardHeight() + 14
	}
	return height
}

func (a *island) batteryRow(c *ui.Context) {
	ui.ScrollHorizontal(c.Key("battery-row")).FillWidth().Height(batteryRowHeight).Shrink(0).
		Margin(0, 0, batteryRowGap).AlignItems(ui.Start).Gap(4).Label("设备电量").Children(func() {
		for _, reading := range a.battery.readings {
			batteryIndicator(c, reading, false)
		}
	})
}

func batteryIndicator(c *ui.Context, reading battery.Reading, compact bool) {
	var itemWidth, itemHeight, ringSize, iconSize, stroke float32 = 50, 44, 28, 14, 2.25
	if compact {
		itemWidth, itemHeight, ringSize, iconSize, stroke = idleBatteryItemWidth, 24, 24, 12, 1.9
	}
	radius := ringSize/2 - 2.5
	name := reading.Name
	display, value := "—", "电量未知"
	known := reading.Percent != battery.UnknownPercent
	if known {
		display = fmt.Sprintf("%d%%", reading.Percent)
		value = display
	}
	icon := batteryIcon
	switch reading.Kind {
	case battery.Computer:
		icon = computerIcon
	case battery.Mouse:
		icon = mouseIcon
	case battery.Keyboard:
		icon = keyboardIcon
	case battery.Trackpad:
		icon = trackpadIcon
	case battery.Headphones:
		icon = headphonesIcon
	}
	switch reading.Part {
	case battery.Left:
		icon, name, display = earbudIcon, name+" 左耳", "左 "+display
	case battery.Right:
		icon, name, display = earbudIcon, name+" 右耳", "右 "+display
	case battery.Case:
		icon, name, display = batteryCaseIcon, name+" 充电盒", "盒 "+display
	}
	color := ink
	if reading.Charging {
		color = chargeGreen
		value += " · 充电中"
	} else if known && reading.Percent <= 20 {
		color = batteryLow
	}
	item := ui.Column(c.Key(reading.ID)).Size(itemWidth, itemHeight).Shrink(0).Gap(2).AlignItems(ui.Center).
		Label(name).Value(value)
	if compact {
		item.Justify(ui.Center)
	}
	if known {
		item.Role(ui.RoleMeter).Range(0, 100, float64(reading.Percent))
	} else {
		item.Role(ui.RoleText).Label(name + " · " + value)
	}
	item.Children(func() {
		ui.Box(c).Size(ringSize, ringSize).Shrink(0).Center().AccessHidden(true).
			Draw(func(p *ui.Painter, r ui.Rect) {
				cx, cy := r.X+r.W/2, r.Y+r.H/2
				var ring ui.Path
				ring.Circle(cx, cy, radius)
				if compact && !known {
					ring.Dash(0, 2, 2)
				}
				p.StrokePath(&ring, stroke, ink.Alpha(0.19))
				if reading.Percent == 100 {
					p.StrokePath(&ring, stroke, color)
				} else if reading.Percent > 0 {
					var arc ui.Path
					arc.MoveTo(cx, cy-radius)
					steps := int(math.Ceil(float64(reading.Percent) * 0.48))
					for i := 1; i <= steps; i++ {
						angle := 2 * math.Pi * float64(reading.Percent*i) / float64(100*steps)
						arc.LineTo(cx+radius*float32(math.Sin(angle)), cy-radius*float32(math.Cos(angle)))
					}
					p.StrokePath(&arc, stroke, color)
				}
			}).Children(func() {
			ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(ink).IgnoreInput(true)
			if reading.Charging {
				ui.Box(c).Absolute().Right(-1).Bottom(0).Size(12, 12).Radius(6).
					Background(chargeGreen).Border(1, white).Center().IgnoreInput(true).Children(func() {
					ui.Icon(c, chargingIcon).Size(9, 9).TextColor(white)
				})
			}
		})
		if !compact {
			ui.Text(c, display).FontSize(11).FontWeight(600).FontFeatures("tnum").SingleLine().
				TextColor(color).AccessHidden(true).IgnoreInput(true)
		}
	})
}
