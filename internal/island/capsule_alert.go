package island

import (
	"fmt"
	"math"
	"time"

	"dynamicisland/internal/battery"
	"github.com/egoist/mygo/ui"
)

type capsuleAlertKind int

const (
	capsuleAlertNone capsuleAlertKind = iota
	capsuleAlertLowBattery
	capsuleAlertTomorrowRain
)

const (
	capsuleAlertEnter    = 320 * time.Millisecond
	capsuleAlertHold     = 4400 * time.Millisecond
	capsuleAlertExit     = 320 * time.Millisecond
	capsuleAlertDuration = capsuleAlertEnter + capsuleAlertHold + capsuleAlertExit
	capsuleAlertWidth    = 200
)

type capsuleAlert struct {
	kind      capsuleAlertKind
	device    battery.Reading
	startedAt time.Time
	until     time.Time
}

func (a *island) showCapsuleAlert(kind capsuleAlertKind, device battery.Reading, now time.Time) {
	a.alert = capsuleAlert{
		kind:      kind,
		device:    device,
		startedAt: now,
		until:     now.Add(capsuleAlertDuration),
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *island) clearCapsuleAlert() {
	a.alert = capsuleAlert{}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *island) capsuleAlertWantsWidth(now time.Time) bool {
	if a.alert.kind == capsuleAlertNone {
		return false
	}
	return now.Before(a.alert.until.Add(-capsuleAlertExit))
}

func (a *island) activeCapsuleAlert(now time.Time, reduceMotion bool) (active bool, progress float32, exiting bool) {
	if a.alert.kind == capsuleAlertNone || !now.Before(a.alert.until) {
		return false, 0, false
	}
	if reduceMotion {
		return true, 1, false
	}
	if elapsed := now.Sub(a.alert.startedAt); elapsed < capsuleAlertEnter {
		p := max(0, min(1, float32(elapsed)/float32(capsuleAlertEnter)))
		return true, ui.EaseOut(p), false
	}
	if remaining := a.alert.until.Sub(now); remaining < capsuleAlertExit {
		p := max(0, min(1, float32(remaining)/float32(capsuleAlertExit)))
		return true, ui.EaseOut(p), true
	}
	return true, 1, false
}

func (a *island) capsuleAlertView(c *ui.Context, opacity, progress float32, exiting bool) {
	now := c.Now()
	reduceMotion := c.Preferences().ReduceMotion
	if !reduceMotion {
		c.AnimationFrame()
	} else if remaining := a.alert.until.Sub(now); remaining > 0 {
		c.After(remaining)
	}
	slideY := 8 * (1 - progress)
	if exiting {
		slideY = -6 * (1 - progress)
	}
	contentWidth := float32(capsuleAlertWidth - 24)
	ui.Box(c.Key("capsule-alert")).Absolute().Left(0).Right(0).Top(0).Height(idleCapsuleHeight).
		Opacity(opacity).Translate(0, slideY).IgnoreInput(true).Center().Children(func() {
		switch a.alert.kind {
		case capsuleAlertTomorrowRain:
			ui.Row(c.Key("alert-rain")).Size(contentWidth, idleCapsuleHeight).Shrink(0).
				Gap(8).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				cloudColor := c.Theme().Text
				dropColor := ui.Hex("#2d7dd2")
				if c.Theme().Dark {
					dropColor = ui.Hex("#64b5f6")
				}
				ui.Box(c.Key("rain-anim-icon")).Size(22, 22).Shrink(0).Label("下雨动画").
					Draw(func(p *ui.Painter, r ui.Rect) {
						paintAnimatedRainIcon(p, r, now, cloudColor, dropColor)
					})
				ui.Text(c.Key("rain-alert-text"), "明日有雨").Shrink(0).SingleLine().
					FontSize(13).FontWeight(600).TextColor(c.Theme().Text)
			})
		case capsuleAlertLowBattery:
			alert := a.alert.device
			lowColor := colorsFor(c.Theme().Dark).lowBattery
			frameColor := c.Theme().Text.Alpha(0.78)
			ui.Row(c.Key("alert-low-battery")).Size(contentWidth, idleCapsuleHeight).Shrink(0).
				Gap(7).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				startedAt := a.alert.startedAt
				ui.Box(c.Key("battery-anim-icon")).Size(24, 16).Shrink(0).Label("低电量动画").
					Draw(func(p *ui.Painter, r ui.Rect) {
						paintDepletingBatteryIcon(p, r, now.Sub(startedAt), lowColor, frameColor)
					})
				ui.Text(c.Key("low-battery-name"), batteryDeviceName(alert)).Grow(1).MinWidth(0).
					SingleLine().Ellipsis("…").FontSize(12).FontWeight(600).TextColor(c.Theme().Text)
				ui.Text(c.Key("low-battery-percent"), fmt.Sprintf("%d%%", alert.Percent)).Shrink(0).
					SingleLine().FontSize(12).FontWeight(650).FontFeatures("tnum").TextColor(lowColor)
			})
		}
	})
}

func paintAnimatedRainIcon(p *ui.Painter, r ui.Rect, now time.Time, cloudColor, dropColor ui.Color) {
	cloudRect := ui.Rect{X: r.X + 1, Y: r.Y - 1, W: r.W - 2, H: r.H * 0.72}
	p.Icon(weatherCloudIcon, cloudRect, cloudColor)

	const cycleMs = 900.0
	basePhase := float64(now.UnixMilli()%int64(cycleMs)) / cycleMs
	if p.Preferences().ReduceMotion {
		basePhase = 0.45
	} else {
		p.AnimationFrame()
	}

	drops := [3]struct {
		xFrac float32
		shift float64
	}{
		{0.32, 0.0},
		{0.52, 0.33},
		{0.72, 0.66},
	}
	startY := r.Y + r.H*0.58
	travel := r.H * 0.34
	dropLen := r.H * 0.17
	for _, d := range drops {
		phase := math.Mod(basePhase+d.shift, 1.0)
		alpha := float32(math.Sin(math.Pi * phase))
		if alpha <= 0.02 {
			continue
		}
		x0 := r.X + r.W*d.xFrac - float32(phase)*1.8
		y0 := startY + float32(phase)*travel
		p.Line(x0, y0, x0-1.1, y0+dropLen, 1.7, dropColor.Alpha(alpha))
	}
}

func paintDepletingBatteryIcon(p *ui.Painter, r ui.Rect, elapsed time.Duration, lowColor, frameColor ui.Color) {
	const (
		drainEnd = 900 * time.Millisecond
		blinkEnd = 2100 * time.Millisecond
	)
	if p.Preferences().ReduceMotion {
		elapsed = blinkEnd
	} else if elapsed < blinkEnd {
		p.AnimationFrame()
	}

	bodyW := r.W - 3.5
	bodyH := r.H - 2
	body := ui.Rect{X: r.X, Y: r.Y + 1, W: bodyW, H: bodyH}
	capRect := ui.Rect{X: body.X + body.W + 1.2, Y: body.Y + body.H*0.30, W: 2.0, H: body.H * 0.40}

	// Drain once from 50% down to 8%, pulse twice at empty, then stay empty without refilling.
	var fillFrac float32 = 0.08
	var fillAlpha float32 = 1
	borderColor := frameColor.Mix(lowColor, 0.85)
	if elapsed < drainEnd {
		t := ui.EaseOut(max(0, float32(elapsed)/float32(drainEnd)))
		fillFrac = 0.50*(1-t) + 0.08*t
		borderColor = frameColor.Mix(lowColor, 0.30+0.55*t)
	} else if elapsed < blinkEnd {
		t := float64(elapsed-drainEnd) / float64(blinkEnd-drainEnd)
		pulse := float32(0.25 + 0.75*math.Abs(math.Cos(t*math.Pi*2)))
		fillAlpha = pulse
		borderColor = frameColor.Mix(lowColor, 0.45+0.55*pulse)
	}

	p.Stroke(body, borderColor, 3.2, 1.5)
	p.Fill(capRect, borderColor, 1)

	pad := float32(2.4)
	innerMaxW := body.W - 2*pad
	innerH := body.H - 2*pad
	if innerMaxW > 1 && innerH > 1 {
		fillW := max(2.2, innerMaxW*fillFrac)
		fillRect := ui.Rect{X: body.X + pad, Y: body.Y + pad, W: fillW, H: innerH}
		p.Fill(fillRect, lowColor.Alpha(fillAlpha), 1.4)
	}
}
