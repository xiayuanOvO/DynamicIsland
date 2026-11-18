package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

type islandFrame struct {
	width, height, open, content float32
}

type islandVelocity struct{ width, height float32 }

type islandAnimation struct {
	from, to          islandFrame
	velocity          islandVelocity
	started, pausedAt time.Time
	duration          time.Duration
}

func (a *island) animatedFrame(c *ui.Context) islandFrame {
	now := c.Now()
	m := &a.animation
	if !a.dragging {
		m.retarget(a.targetFrame(), now)
	}
	frame, _ := m.sample(now)
	if m.pausedAt.IsZero() && now.Sub(m.started) < m.duration {
		c.AnimationFrame()
	}
	return frame
}

func (a *island) targetFrame() islandFrame {
	if a.expanded {
		return islandFrame{width: panelWidth, height: float32(a.panelHeight()), open: 1, content: 1}
	}
	width := float32(compactWidth)
	if a.compactIdle() {
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
	switch {
	case target.open == frame.open:
		m.duration = 300 * time.Millisecond
	case target.open > frame.open:
		m.duration = 360 * time.Millisecond
	default:
		m.duration = 280 * time.Millisecond
	}
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
	widthFrequency, heightFrequency := 36.0, 36.0
	widthDamping, heightDamping := 0.86, 0.86
	revealDuration := 160 * time.Millisecond
	if m.to.open > m.from.open {
		widthFrequency, heightFrequency = 32, 34
		widthDamping, heightDamping = 0.74, 0.82
		revealDuration = 180 * time.Millisecond
	} else if m.to.open < m.from.open {
		widthFrequency, heightFrequency = 40, 44
		widthDamping, heightDamping = 0.80, 0.88
		revealDuration = 140 * time.Millisecond
	}
	width, vx := springSample(m.from.width, m.velocity.width, m.to.width, elapsed.Seconds(), widthFrequency, widthDamping)
	height, vy := springSample(m.from.height, m.velocity.height, m.to.height, elapsed.Seconds(), heightFrequency, heightDamping)
	t := min(float32(1), float32(elapsed)/float32(revealDuration))
	content := ui.EaseOut(t)
	if m.to.open > m.from.open {
		content = ui.EaseOut(animationPhase(t, 0.06, 0.90))
	} else if m.to.open < m.from.open {
		content = ui.EaseOut(animationPhase(t, 0, 0.65))
	}
	return islandFrame{
		width:   max(compactHeight, width),
		height:  max(compactHeight-4, height),
		open:    m.from.open + (m.to.open-m.from.open)*ui.EaseOut(t),
		content: m.from.content + (m.to.content-m.from.content)*content,
	}, islandVelocity{width: vx, height: vy}
}

// Solve a damped spring analytically so interrupted transitions retain
// their velocity and follow the same path at any frame rate.
func springSample(from, velocity, target float32, seconds, frequency, damping float64) (float32, float32) {
	x, v := float64(from-target), float64(velocity)
	decayRate := damping * frequency
	oscillation := frequency * math.Sqrt(1-damping*damping)
	sin, cos := math.Sincos(oscillation * seconds)
	decay := math.Exp(-decayRate * seconds)
	position := (x*cos + (v+decayRate*x)/oscillation*sin) * decay
	speed := (v*cos - (frequency*frequency*x+decayRate*v)/oscillation*sin) * decay
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
	}
}
