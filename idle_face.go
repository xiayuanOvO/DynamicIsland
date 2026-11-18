package main

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type idleMood uint8

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

func (a *island) drawIdleFace(c *ui.Context, height, open, offset float32) {
	face := &a.face
	eyeHeight, amount := float32(13), float32(0)
	if open == 0 && !a.expanded && !a.moving {
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
			eyeHeight -= 11 * ui.EaseInOut(float32(elapsed)/float32(closeTime))
			c.AnimationFrame()
		default:
			eyeHeight = 2 + 11*ui.EaseInOut(float32(elapsed-closeTime)/float32(openTime))
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
				ball := a.windowBounds(a.width, a.height)
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
	ui.Box(c.Key("idle-face")).Absolute().Left(0).Right(0).Top(0).Height(height).
		Opacity(1-open).Translate(offset, 0).IgnoreInput(true).Label("空闲表情").
		Draw(func(p *ui.Painter, r ui.Rect) {
			paintIdleFace(p, r, mood, amount, eyeHeight, gaze)
		})
}

func paintIdleFace(p *ui.Painter, r ui.Rect, mood idleMood, amount, eyeHeight float32, gaze ui.Point) {
	if amount == 0 {
		mood = moodNeutral
	}
	centerX, centerY := r.X+r.W/2, r.Y+r.H/2-4
	stroke := ink.Alpha(0.76)
	for index, offset := range [...]float32{-6.5, 6.5} {
		x, y := centerX+offset, centerY
		width, height, opacity := float32(7), eyeHeight, float32(1)
		switch mood {
		case moodSleepy:
			height *= 1 - 0.72*amount
			y += 1.4 * amount
		case moodBored:
			height *= 1 - (0.52+0.16*float32(index))*amount
			x -= 0.8 * amount
			y += 1.1 * amount
		case moodHappy:
			opacity -= amount
			var eyebrow ui.Path
			eyebrow.MoveTo(x-3, y+1).QuadTo(x, y-5, x+3, y+1)
			p.StrokePath(&eyebrow, 1.6, stroke.Alpha(amount))
		case moodLooking:
			x += gaze.X * 1.8 * amount
			y += gaze.Y * 1.4 * amount
			width += 0.8 * amount
		}
		height = max(1.6, height)
		eye := ui.Rect{X: x - width/2, Y: y - height/2, W: width, H: height}
		p.Stroke(eye, stroke.Alpha(opacity), min(width/2, height/2), 1.5)
		if mood == moodLooking && height >= 5 {
			p.Fill(ui.Rect{X: x - 0.9 + gaze.X*1.25, Y: y - 0.9 + gaze.Y*height*0.22, W: 1.8, H: 1.8}, stroke.Alpha(amount), 0.9)
		}
	}

	mouth := stroke.Alpha(amount)
	switch mood {
	case moodSleepy:
		height := 2 + 4*amount
		p.Stroke(ui.Rect{X: centerX - 2.3, Y: centerY + 9, W: 4.6, H: height}, mouth, min(2.3, height/2), 1.3)
	case moodBored:
		p.Line(centerX-2.5, centerY+11, centerX+2.5, centerY+11, 1.4, mouth)
	case moodHappy:
		var smile ui.Path
		smile.MoveTo(centerX-4.6, centerY+8.5).QuadTo(centerX, centerY+15.5, centerX+4.6, centerY+8.5)
		p.StrokePath(&smile, 1.5, mouth)
	}
}
