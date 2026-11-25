package island

import (
	"context"
	"fmt"
	"math"
	"time"

	"dynamicisland/internal/battery"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	batteryRowHeight         = 50
	batteryRowGap            = 10
	lowBatteryThreshold      = 15
	lowBatteryResetThreshold = 20
)

type islandBattery struct {
	readings    []battery.Reading
	refreshAt   time.Time
	reading     bool
	notifiedLow map[string]bool
	notifyLow   func(battery.Reading)
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
)

func (a *island) refreshBattery() {
	b := &a.battery
	if !battery.Supported || b.reading || a.win == nil {
		return
	}
	b.reading = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		snapshot := battery.ReadIncremental(ctx, func(partial battery.Snapshot) {
			a.win.Update(func() {
				b.readings = partial.Merge(b.readings)
				a.checkLowBattery(time.Now())
			})
		})
		a.win.Update(func() {
			b.reading = false
			b.readings = snapshot.Merge(b.readings)
			a.checkLowBattery(time.Now())
			b.refreshAt = time.Now().Add(30 * time.Second)
		})
	}()
}

func (a *island) updateBattery(c *ui.Context) {
	b := &a.battery
	showOverview := a.expanded && !a.todos.open && !a.clipboard.open
	showIdle := a.showsIdleCapsule() && a.settings.showsIdleItem(idleItemBattery)
	watchLow := a.settings.LowBatteryAlert && a.media.track.Playing && a.media.track.Title != ""
	if !battery.Supported || (!showOverview && !showIdle && !watchLow) || !a.win.IsVisible() || b.reading {
		return
	}
	if delay := b.refreshAt.Sub(c.Now()); delay > 0 {
		c.After(delay)
		return
	}
	a.refreshBattery()
}

func batteryDeviceName(reading battery.Reading) string {
	switch reading.Part {
	case battery.Left:
		return reading.Name + " 左耳"
	case battery.Right:
		return reading.Name + " 右耳"
	case battery.Case:
		return reading.Name + " 充电盒"
	default:
		return reading.Name
	}
}

func batteryDeviceIcon(reading battery.Reading) *ui.SVG {
	switch reading.Part {
	case battery.Left, battery.Right:
		return earbudIcon
	case battery.Case:
		return batteryCaseIcon
	}
	switch reading.Kind {
	case battery.Computer:
		return computerIcon
	case battery.Mouse:
		return mouseIcon
	case battery.Keyboard:
		return keyboardIcon
	case battery.Trackpad:
		return trackpadIcon
	case battery.Headphones:
		return headphonesIcon
	default:
		return batteryIcon
	}
}

func (a *island) checkLowBattery(now time.Time) {
	b := &a.battery
	if !a.settings.LowBatteryAlert {
		return
	}
	if b.notifiedLow == nil {
		b.notifiedLow = make(map[string]bool)
	}
	for _, reading := range b.readings {
		if reading.Kind == battery.Computer || reading.Percent == battery.UnknownPercent {
			continue
		}
		key := fmt.Sprintf("%s:%d", reading.ID, reading.Part)
		if reading.Charging || reading.Percent > lowBatteryResetThreshold {
			delete(b.notifiedLow, key)
			continue
		}
		if reading.Percent <= lowBatteryThreshold && !b.notifiedLow[key] {
			b.notifiedLow[key] = true
			r := reading
			a.showCapsuleAlert(capsuleAlertLowBattery, r, now)
			if b.notifyLow != nil {
				b.notifyLow(r)
			} else {
				name := batteryDeviceName(r)
				notice := mygo.NewNotification(mygo.NotificationOptions{
					ID: "battery-" + key, Group: "battery",
					Title: name + " 电量低",
					Body:  fmt.Sprintf("剩余电量 %d%%", r.Percent),
				})
				go func() { _ = notice.Show() }()
			}
		}
	}
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
	a.batteryWidget(c, widgetSizeMedium, panelBodyWidth, batteryRowHeight)
}

func (a *island) batteryWidget(c *ui.Context, size widgetSize, width, height float32) {
	switch size {
	case widgetSizeSmall:
		a.batterySmallCard(c, width, height)
	case widgetSizeLarge:
		a.batteryLargeCard(c, width, height)
	default:
		ui.ScrollHorizontal(c.Key("battery-row")).Size(width, height).Shrink(0).
			AlignItems(ui.Start).Gap(4).Label("设备电量").Children(func() {
			for _, reading := range a.battery.readings {
				batteryIndicator(c, reading, false)
			}
		})
	}
}

func (a *island) batterySmallCard(c *ui.Context, width, height float32) {
	tint := colorsFor(c.Theme().Dark).glass
	card := ui.Column(c.Key("battery-small")).Size(width, height).Shrink(0).Radius(13).Clip().
		Padding(10, 12).Justify(ui.Center).Gap(8).
		Background(tint.Alpha(0.4)).Border(0.75, tint.Alpha(0.65)).Label("设备电量")
	card.Children(func() {
		for row := range 2 {
			ui.Row(c.Key(row)).FillWidth().Justify(ui.SpaceAround).AlignItems(ui.Center).Children(func() {
				for col := range 2 {
					idx := row*2 + col
					if idx < len(a.battery.readings) {
						batterySmallCell(c, a.battery.readings[idx])
					} else {
						batteryEmptyRing(c)
					}
				}
			})
		}
	})
}

func batterySmallCell(c *ui.Context, reading battery.Reading) {
	const ringSize, iconSize, stroke float32 = 38, 16, 3.0
	radius := ringSize/2 - 3
	name := batteryDeviceName(reading)
	display, value := "—", "电量未知"
	known := reading.Percent != battery.UnknownPercent
	if known {
		display = fmt.Sprintf("%d%%", reading.Percent)
		value = display
	}
	text := c.Theme().Text
	colors := colorsFor(c.Theme().Dark)
	color := colors.charging
	if reading.Charging {
		value += " · 充电中"
	} else if known && reading.Percent <= 20 {
		color = colors.lowBattery
	} else if !known {
		color = text.Alpha(0.45)
	}
	cell := ui.Column(c.Key(reading.ID)).Size(72, 54).Shrink(0).Gap(2).AlignItems(ui.Center).Justify(ui.Center).
		Label(name).Value(value)
	if known {
		cell.Role(ui.RoleMeter).Range(0, 100, float64(reading.Percent))
	} else {
		cell.Role(ui.RoleText).Label(name + " · " + value)
	}
	cell.Children(func() {
		ui.Box(c).Size(ringSize, ringSize).Shrink(0).Center().AccessHidden(true).
			Draw(func(p *ui.Painter, r ui.Rect) {
				drawBatteryRing(p, r, radius, stroke, reading.Percent, known, color, text, false)
			}).Children(func() {
			ui.Icon(c, batteryDeviceIcon(reading)).Size(iconSize, iconSize).TextColor(text).IgnoreInput(true)
			if reading.Charging {
				ui.Box(c).Absolute().Right(-1).Bottom(0).Size(13, 13).Radius(6.5).
					Background(lightColors.charging).Border(1, white).Center().IgnoreInput(true).Children(func() {
					ui.Icon(c, chargingIcon).Size(9, 9).TextColor(white)
				})
			}
		})
		ui.Text(c, display).FontSize(10.5).FontWeight(600).FontFeatures("tnum").SingleLine().
			TextColor(text.Alpha(0.85)).AccessHidden(true).IgnoreInput(true)
	})
}

func batteryEmptyRing(c *ui.Context) {
	const ringSize, stroke float32 = 38, 3.0
	radius := ringSize/2 - 3
	text := c.Theme().Text
	ui.Column(c).Size(72, 54).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).AccessHidden(true).Children(func() {
		ui.Box(c).Size(ringSize, ringSize).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			var ring ui.Path
			ring.Circle(cx, cy, radius)
			p.StrokePath(&ring, stroke, text.Alpha(0.12))
		})
	})
}

func (a *island) batteryLargeCard(c *ui.Context, width, height float32) {
	tint := colorsFor(c.Theme().Dark).glass
	text := c.Theme().Text
	colors := colorsFor(c.Theme().Dark)
	card := ui.Column(c.Key("battery-large")).Size(width, height).Shrink(0).Radius(13).Clip().
		Padding(9, 12).Gap(6).
		Background(tint.Alpha(0.4)).Border(0.75, tint.Alpha(0.65)).Label("设备电量")
	card.Children(func() {
		ui.Row(c.Key("battery-large-heading")).FillWidth().Height(todoPreviewHeader).Shrink(0).
			Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, batteryIcon).Size(14, 14).TextColor(c.Theme().Accent)
			ui.Text(c, "设备电量").Grow(1).SingleLine().FontSize(11).FontWeight(600).TextColor(c.Theme().TextMuted)
		})
		ui.Scroll(c.Key("battery-large-list")).FillWidth().Grow(1).MinHeight(0).Gap(6).Children(func() {
			for _, reading := range a.battery.readings {
				known := reading.Percent != battery.UnknownPercent
				display, value := "—", "电量未知"
				if known {
					display = fmt.Sprintf("%d%%", reading.Percent)
					value = display
				}
				barColor := colors.charging
				if reading.Charging {
					value += " · 充电中"
				} else if known && reading.Percent <= 20 {
					barColor = colors.lowBattery
				} else if !known {
					barColor = text.Alpha(0.25)
				}
				name := batteryDeviceName(reading)
				row := ui.Row(c.Key(reading.ID)).FillWidth().Height(32).Shrink(0).Gap(10).AlignItems(ui.Center).
					Label(name).Value(value)
				if known {
					row.Role(ui.RoleMeter).Range(0, 100, float64(reading.Percent))
				} else {
					row.Role(ui.RoleText).Label(name + " · " + value)
				}
				row.Children(func() {
					ui.Box(c).Size(24, 24).Shrink(0).Center().AccessHidden(true).
						Draw(func(p *ui.Painter, r ui.Rect) {
							drawBatteryRing(p, r, 9.5, 2.0, reading.Percent, known, barColor, text, false)
						}).Children(func() {
						ui.Icon(c, batteryDeviceIcon(reading)).Size(12, 12).TextColor(text).IgnoreInput(true)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(4).AccessHidden(true).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").FontSize(12).FontWeight(550).TextColor(text)
							if reading.Charging {
								ui.Icon(c, chargingIcon).Size(11, 11).TextColor(colors.charging)
							}
							ui.Text(c, display).Shrink(0).FontSize(11.5).FontWeight(600).FontFeatures("tnum").TextColor(barColor)
						})
						pct := float32(0)
						if known {
							pct = float32(reading.Percent) / 100
						}
						ui.Box(c).FillWidth().Height(4).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
							p.Fill(r, text.Alpha(0.14), 2)
							if pct > 0 {
								fill := ui.Rect{X: r.X, Y: r.Y, W: r.W * min(1, max(0, pct)), H: r.H}
								p.Fill(fill, barColor, min(2, fill.W/2))
							}
						})
					})
				})
			}
		})
	})
}

func drawBatteryRing(p *ui.Painter, r ui.Rect, radius, stroke float32, percent int, known bool, color, text ui.Color, dashUnknown bool) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	var ring ui.Path
	ring.Circle(cx, cy, radius)
	if dashUnknown && !known {
		ring.Dash(0, 2, 2)
	}
	p.StrokePath(&ring, stroke, text.Alpha(0.19))
	if percent == 100 {
		p.StrokePath(&ring, stroke, color)
	} else if percent > 0 {
		var arc ui.Path
		arc.MoveTo(cx, cy-radius)
		steps := int(math.Ceil(float64(percent) * 0.48))
		for i := 1; i <= steps; i++ {
			angle := 2 * math.Pi * float64(percent*i) / float64(100*steps)
			arc.LineTo(cx+radius*float32(math.Sin(angle)), cy-radius*float32(math.Cos(angle)))
		}
		p.StrokePath(&arc, stroke, color)
	}
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
	text := c.Theme().Text
	colors := colorsFor(c.Theme().Dark)
	color := text
	if reading.Charging {
		color = colors.charging
		value += " · 充电中"
	} else if known && reading.Percent <= 20 {
		color = colors.lowBattery
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
				p.StrokePath(&ring, stroke, text.Alpha(0.19))
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
			ui.Icon(c, icon).Size(iconSize, iconSize).TextColor(text).IgnoreInput(true)
			if reading.Charging {
				ui.Box(c).Absolute().Right(-1).Bottom(0).Size(12, 12).Radius(6).
					Background(lightColors.charging).Border(1, white).Center().IgnoreInput(true).Children(func() {
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
