package island

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type idleMood uint8

const idleEyeHeight float32 = 13

const (
	moodNeutral idleMood = iota
	moodSleepy
	moodBored
	moodHappy
	moodLooking
)

type idleFaceState struct {
	mood    idleMood
	moodAt  time.Time
	blinkAt time.Time
	gaze    ui.Point
	gazeAt  time.Time
}

func (f *idleFaceState) lookAt(now time.Time, delta ui.Point) {
	distance := max(72, math.Hypot(float64(delta.X), float64(delta.Y)))
	elapsed := now.Sub(f.gazeAt)
	if f.gazeAt.IsZero() {
		elapsed = time.Second / 30
	}
	f.gazeAt = now
	blend := float32(1 - math.Exp(-max(0, elapsed.Seconds())/0.095))
	f.gaze.X += (delta.X/float32(distance) - f.gaze.X) * blend
	f.gaze.Y += (delta.Y/float32(distance) - f.gaze.Y) * blend
}

func (a *island) drawIdleFace(c *ui.Context, slot ui.Element, opacity, offset float32) {
	face := &a.face
	const closedEyeHeight = 2
	eyeHeight, amount := idleEyeHeight, float32(0)
	if opacity == 1 && !a.expanded && !a.moving {
		const closeTime, openTime = 72 * time.Millisecond, 128 * time.Millisecond
		now := c.Now()
		if face.blinkAt.IsZero() || !now.Before(face.blinkAt.Add(closeTime+openTime)) {
			face.blinkAt = now.Add(6*time.Second + time.Duration(rand.IntN(5000))*time.Millisecond)
		}
		elapsed := now.Sub(face.blinkAt)
		switch {
		case elapsed < 0:
			c.After(-elapsed)
		case elapsed < closeTime:
			eyeHeight -= (idleEyeHeight - closedEyeHeight) * ui.EaseInOut(float32(elapsed)/float32(closeTime))
			c.AnimationFrame()
		default:
			eyeHeight = closedEyeHeight + (idleEyeHeight-closedEyeHeight)*ui.EaseInOut(float32(elapsed-closeTime)/float32(openTime))
			c.AnimationFrame()
		}

		const duration, rise, fall = 3200 * time.Millisecond, 280 * time.Millisecond, 420 * time.Millisecond
		if face.moodAt.IsZero() || !now.Before(face.moodAt.Add(duration)) {
			next := moodSleepy + idleMood(rand.IntN(int(moodLooking)))
			if next == face.mood {
				next = next%moodLooking + moodSleepy
			}
			face.mood, face.moodAt = next, now.Add(time.Duration(10+rand.IntN(13))*time.Second)
			face.gaze, face.gazeAt = ui.Point{}, time.Time{}
		}
		elapsed = now.Sub(face.moodAt)
		if elapsed < 0 {
			c.After(-elapsed)
		} else {
			amount = ui.EaseInOut(min(1, float32(elapsed)/float32(rise), float32(duration-elapsed)/float32(fall)))
			if elapsed < rise || elapsed >= duration-fall {
				c.AnimationFrame()
			} else {
				c.After(duration - fall - elapsed)
			}
			if face.mood == moodLooking && a.win != nil {
				cursor := mygo.Screen.CursorScreenPoint()
				ball := a.windowBounds(int(a.shownFrame.width), int(a.shownFrame.height))
				face.lookAt(now, ui.Point{
					X: float32(cursor.X-ball.X-ball.Width/2) - offset,
					Y: float32(cursor.Y - ball.Y - ball.Height/2 + 4),
				})
				c.After(time.Second / 30)
			}
		}
	} else {
		*face = idleFaceState{}
	}
	mood, gaze := face.mood, face.gaze
	slot.IgnoreInput(true).Label("空闲表情").
		Draw(func(p *ui.Painter, r ui.Rect) {
			paintIdleFace(p, r, mood, amount, eyeHeight, gaze)
		})
}

func paintIdleFace(p *ui.Painter, r ui.Rect, mood idleMood, amount, eyeHeight float32, gaze ui.Point) {
	const ellipseKappa = 0.55228475
	centerX, centerY := r.X+r.W/2, r.Y+r.H/2-4
	eyeColor := ui.RGB(48, 48, 48)
	for index, offset := range [...]float32{-6.5, 6.5} {
		x, y := centerX+offset, centerY
		radiusX := float32(4.5)
		upper, lower := eyeHeight/2, eyeHeight/2
		var lowerCurve float32
		switch mood {
		case moodSleepy:
			upper -= eyeHeight * 0.445 * amount
			lower -= eyeHeight * 0.27 * amount
			y += 2 * amount
		case moodBored:
			upper -= eyeHeight * (0.23 + 0.16*float32(index)) * amount
			x -= 0.8 * amount
		case moodHappy:
			upper -= eyeHeight * 0.14 * amount
			lower -= eyeHeight * 0.62 * amount
			// Lift the bottom through the center while keeping the tips rounded.
			lowerCurve = eyeHeight * 0.19 * amount
		case moodLooking:
			x += gaze.X * 2.5 * amount
			y += gaze.Y * 2.0 * amount
			radiusX += 0.4 * amount
		}

		left, right := x-radiusX, x+radiusX
		top, bottom := y-upper, y+lower
		handleX, upperSide := radiusX*ellipseKappa, upper*ellipseKappa
		lowerSide := lower*ellipseKappa + lowerCurve
		var eye ui.Path
		eye.MoveTo(left, y).
			CubeTo(left, y-upperSide, x-handleX, top, x, top).
			CubeTo(x+handleX, top, right, y-upperSide, right, y).
			CubeTo(right, y+lowerSide, x+handleX, bottom, x, bottom).
			CubeTo(x-handleX, bottom, left, y+lowerSide, left, y).
			Close()
		p.FillPath(&eye, eyeColor)
	}
}
