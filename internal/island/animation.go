package island

import (
	"math"
	"time"

	"github.com/egoist/mygo"
)

const (
	islandResizeDuration = 440 * time.Millisecond
	windowFrameInterval  = time.Second / 60
)

type islandFrame struct {
	width, height, open, content, todo, editor, schedule, clipboard float32
}

func (f islandFrame) aligned() islandFrame {
	// The native window uses integral bounds. Keep both sides of the
	// centered capsule the same distance from its anchor as it resizes.
	f.width = 2 * float32(math.Round(float64(f.width)/2))
	f.height = float32(math.Round(float64(f.height)))
	return f
}

// Keep the native drawing surface at one size while the visible island
// and its backdrop animate inside it.
type transitionCanvas struct {
	bounds  mygo.Rectangle
	warming bool
	reshape func(mygo.Rectangle, int)
	restore func()
}

func (c transitionCanvas) contains(r mygo.Rectangle) bool {
	return r.X >= c.bounds.X && r.Y >= c.bounds.Y &&
		r.X+r.Width <= c.bounds.X+c.bounds.Width && r.Y+r.Height <= c.bounds.Y+c.bounds.Height
}

func (c transitionCanvas) contentRect(r mygo.Rectangle) mygo.Rectangle {
	r.X -= c.bounds.X
	r.Y -= c.bounds.Y
	return r
}

type islandVelocity struct{ width, height, open, todo, editor, schedule, clipboard float32 }

type islandAnimation struct {
	from, to          islandFrame
	velocity          islandVelocity
	started, pausedAt time.Time
	lastTick          time.Time
	duration          time.Duration
}

func (a *island) scheduleWindowFrame(deadline time.Time) {
	if a.windowFramePending || a.dragging || a.moving || a.animation.to == a.shownFrame {
		return
	}
	a.windowFramePending = true
	if a.wakeWindowDisplayLink != nil {
		a.wakeWindowDisplayLink()
		return
	}
	// Apply bounds before requesting a paint. Resizing from a paint queues
	// a second pass at a different size and can stretch the previous frame.
	time.AfterFunc(time.Until(deadline), func() {
		mygo.RunOnMain(func() {
			active := a.advanceWindowFrame(time.Now())
			a.windowFramePending = false
			if active {
				now := time.Now()
				delay := windowFrameInterval - max(0, now.Sub(deadline))%windowFrameInterval
				a.scheduleWindowFrame(now.Add(delay))
			}
		})
	})
}

func (a *island) advanceWindowFrame(now time.Time) bool {
	if a.dragging || a.moving || !a.win.IsVisible() {
		return false
	}
	if a.canvas.warming {
		a.animation.started, a.animation.lastTick = now, now
		return true
	}
	frame := a.animation.advance(now)
	frame = frame.aligned()
	if frame != a.shownFrame {
		a.applyWindowFrame(frame, true)
		a.win.Invalidate()
	}
	return a.shownFrame != a.animation.to
}

func (a *island) targetFrame() islandFrame {
	if a.expanded {
		frame := islandFrame{width: panelWidth, height: float32(a.panelHeight()), open: 1, content: 1}
		if a.todos.open {
			frame.todo = 1
		}
		if a.clipboard.open {
			frame.clipboard = 1
		}
		if a.todos.draft != nil {
			frame.editor = 1
		}
		draft := a.todos.draft
		if draft == nil {
			draft = a.todos.leaving
		}
		if draft != nil && draft.scheduled {
			frame.schedule = 1
		}
		return frame
	}
	width := float32(compactWidth)
	if a.dock.side != dockNone || a.compactIdle() {
		width = compactHeight
	} else if a.media.track.Playing && a.media.track.Title != "" {
		width = compactMediaWidth
	}
	return islandFrame{width: width, height: compactHeight}
}

func (m *islandAnimation) retarget(target islandFrame, now time.Time) {
	if m.started.IsZero() {
		m.from = islandFrame{width: compactWidth, height: compactHeight}
		m.to, m.started = m.from, now
	}
	if target == m.to {
		return
	}
	frame, velocity := m.sample(now)
	m.from, m.to, m.velocity, m.started = frame, target, velocity, now
	m.lastTick = now
	m.duration = islandResizeDuration
	if target.open != frame.open {
		m.duration = 610 * time.Millisecond
	}
}

func (m *islandAnimation) advance(now time.Time) islandFrame {
	// If the main thread misses a refresh, continue from the last visible
	// position instead of moving multiple frames' distance in a single step.
	if !m.lastTick.IsZero() {
		m.started = m.started.Add(max(0, now.Sub(m.lastTick)-windowFrameInterval))
	}
	m.lastTick = now
	frame, _ := m.sample(now)
	return frame
}

func (m *islandAnimation) sample(now time.Time) (islandFrame, islandVelocity) {
	if m.duration == 0 {
		return m.to, islandVelocity{}
	}
	if !m.pausedAt.IsZero() {
		now = m.pausedAt
	}
	elapsed := max(time.Duration(0), now.Sub(m.started))
	if elapsed >= m.duration {
		return m.to, islandVelocity{}
	}
	frequency := 19.0
	if m.to.open != m.from.open {
		frequency = 14
	}
	seconds := elapsed.Seconds()
	width, vx := springSample(m.from.width, m.velocity.width, m.to.width, seconds, frequency)
	height, vy := springSample(m.from.height, m.velocity.height, m.to.height, seconds, frequency)
	todo, vt := springSample(m.from.todo, m.velocity.todo, m.to.todo, seconds, frequency)
	editor, ve := springSample(m.from.editor, m.velocity.editor, m.to.editor, seconds, frequency)
	schedule, vs := springSample(m.from.schedule, m.velocity.schedule, m.to.schedule, seconds, frequency)
	clipboard, vc := springSample(m.from.clipboard, m.velocity.clipboard, m.to.clipboard, seconds, frequency)
	open, vo := springSample(m.from.open, m.velocity.open, m.to.open, seconds, frequency)
	content := animationPhase(open, 0.20, 0.85)
	return islandFrame{
		width:     max(compactHeight, width),
		height:    max(compactHeight-4, height),
		open:      min(1, max(0, open)),
		content:   content,
		todo:      min(1, max(0, todo)),
		editor:    editor,
		schedule:  schedule,
		clipboard: min(1, max(0, clipboard)),
	}, islandVelocity{width: vx, height: vy, open: vo, todo: vt, editor: ve, schedule: vs, clipboard: vc}
}

// A critically damped spring eases into the target and preserves velocity
// when the target changes mid-animation.
func springSample(from, velocity, target float32, seconds, frequency float64) (float32, float32) {
	x, v := float64(from-target), float64(velocity)
	slope := v + frequency*x
	decay := math.Exp(-frequency * seconds)
	position := (x + slope*seconds) * decay
	speed := (v - frequency*slope*seconds) * decay
	return target + float32(position), float32(speed)
}

func animationPhase(t, start, end float32) float32 {
	return min(1, max(0, (t-start)/(end-start)))
}

func (m *islandAnimation) pause(now time.Time) {
	if m.pausedAt.IsZero() {
		m.pausedAt = now
	}
}

func (m *islandAnimation) resume(now time.Time) {
	if !m.pausedAt.IsZero() {
		m.started = m.started.Add(now.Sub(m.pausedAt))
		m.pausedAt = time.Time{}
		m.lastTick = now
	}
}
