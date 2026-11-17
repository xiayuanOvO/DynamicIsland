package main

import (
	"math"
	"testing"
	"time"
)

func TestAnimationRespondsQuicklyWithSmallRecoil(t *testing.T) {
	now := time.Unix(1, 0)
	compact := islandFrame{width: compactWidth, height: compactHeight}
	expanded := islandFrame{width: panelWidth, height: 541, open: 1, content: 1}
	var motion islandAnimation
	motion.retarget(expanded, now)
	frame, _ := motion.sample(now.Add(80 * time.Millisecond))
	if frame.width < compact.width+(expanded.width-compact.width)*0.7 ||
		frame.height < compact.height+(expanded.height-compact.height)*0.7 {
		t.Fatalf("opening is still lagging at 80 ms: %+v", frame)
	}
	frame, _ = motion.sample(now.Add(100 * time.Millisecond))
	if frame.content < 0.8 {
		t.Fatalf("content is not ready for input at 100 ms: %+v", frame)
	}
	peak := float32(0)
	for elapsed := time.Duration(0); elapsed <= 400*time.Millisecond; elapsed += time.Millisecond {
		frame, _ = motion.sample(now.Add(elapsed))
		peak = max(peak, frame.width)
	}
	if peak <= expanded.width || peak > expanded.width+8 {
		t.Fatalf("opening recoil is missing or too large: %f", peak)
	}
	motion.retarget(compact, now.Add(time.Second))
	frame, _ = motion.sample(now.Add(time.Second + 80*time.Millisecond))
	if frame.width > compact.width+(expanded.width-compact.width)*0.15 {
		t.Fatalf("closing is still lagging at 80 ms: %+v", frame)
	}
	frame, velocity := motion.sample(now.Add(time.Second + 300*time.Millisecond))
	if frame != compact || velocity != (islandVelocity{}) {
		t.Fatalf("closing tail did not settle: %+v, %+v", frame, velocity)
	}
}

func TestAnimationRetargetKeepsPositionAndVelocity(t *testing.T) {
	now := time.Unix(1, 0)
	expanded := islandFrame{width: panelWidth, height: 541, open: 1, content: 1}
	var motion islandAnimation
	motion.retarget(expanded, now)
	switched := now.Add(90 * time.Millisecond)
	before, speedBefore := motion.sample(switched)
	motion.retarget(expanded, switched)
	if motion.started != now {
		t.Fatal("repeating an open request restarted the animation")
	}
	motion.retarget(islandFrame{width: compactWidth, height: compactHeight}, switched)
	after, speedAfter := motion.sample(switched)
	if before != after || math.Abs(float64(speedBefore.width-speedAfter.width)) > 0.01 ||
		math.Abs(float64(speedBefore.height-speedAfter.height)) > 0.01 {
		t.Fatalf("retarget jumped: %+v / %+v -> %+v / %+v", before, speedBefore, after, speedAfter)
	}
	after, _ = motion.sample(switched.Add(80 * time.Millisecond))
	if after.width >= before.width || after.height >= before.height {
		t.Fatalf("reversal is still following the previous target: %+v -> %+v", before, after)
	}
}

func TestAnimationDragPausePreservesRemainingMotion(t *testing.T) {
	now := time.Unix(1, 0)
	var motion islandAnimation
	motion.retarget(islandFrame{width: panelWidth, height: 541, open: 1, content: 1}, now)
	paused := now.Add(80 * time.Millisecond)
	before, _ := motion.sample(paused)
	motion.pause(paused)
	resumed := paused.Add(time.Second)
	during, _ := motion.sample(resumed)
	motion.resume(resumed)
	after, _ := motion.sample(resumed)
	if during != before || after != before {
		t.Fatalf("dragging advanced the animation: %+v -> %+v -> %+v", before, during, after)
	}
	frame, _ := motion.sample(resumed.Add(80 * time.Millisecond))
	if frame == before || frame == motion.to {
		t.Fatalf("remaining motion did not resume: %+v", frame)
	}
}
