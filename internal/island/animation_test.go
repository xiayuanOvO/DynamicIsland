package island

import (
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

func TestTransitionGlassCoversCanvasBeyondCapsule(t *testing.T) {
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		capsule := ui.Box(c).Absolute().Left(91).Top(0).Size(246, idleCapsuleHeight).
			Radius(islandCornerRadius(idleCapsuleHeight)).Clip()
		glassSurface(c, capsule, true, true)
	}, panelWidth, 154, 1)
	before := img.RGBAAt(panelWidth/2, idleCapsuleHeight-1)
	after := img.RGBAAt(panelWidth/2, idleCapsuleHeight+1)
	if after.A == 0 || max(before.A, after.A)-min(before.A, after.A) > 3 {
		t.Fatalf("the glass tint should continue past the previous capsule edge: above %v, below %v", before, after)
	}
}

func TestIdleCapsuleHeightAnimation(t *testing.T) {
	a := island{}
	a.expanded = true
	from := a.targetFrame()
	a.expanded = false
	to := a.targetFrame()
	now := time.Unix(0, 0)
	animation := islandAnimation{from: from, to: from, started: now}
	animation.retarget(to, now)
	frame, _ := animation.sample(now.Add(animation.duration - time.Millisecond))
	if frame.aligned().height != idleCapsuleHeight {
		t.Fatalf("the animation should approach the shorter idle height before it settles, got %.2f", frame.height)
	}
}

func TestClipboardAndOverviewShareCanvasBounds(t *testing.T) {
	a := island{
		centerX: 600,
		top:     40,
	}
	a.clipboard.Recording = true
	a.media.track = nowplaying.Track{Title: "示例曲目"}
	compact := a.targetFrame()
	compactBounds := a.windowBounds(int(compact.width), int(compact.height))
	a.expanded = true
	overview := a.targetFrame()
	a.clipboard.open = true
	clipboard := a.targetFrame()

	// Either page can be the first one opened from the compact island.
	for _, first := range []islandFrame{overview, clipboard} {
		a.animation.to = first
		canvas := transitionCanvas{bounds: a.transitionCanvasBounds(compactBounds, true)}
		for _, target := range []islandFrame{overview, clipboard} {
			a.animation.to = target
			bounds := a.windowBounds(int(target.width), int(target.height))
			if !canvas.contains(bounds) || a.transitionCanvasBounds(canvas.bounds, true) != canvas.bounds {
				t.Fatalf("the canvas %+v should cover both pages without resizing", canvas.bounds)
			}
			if origin := canvas.contentRect(bounds); origin.X != 0 || origin.Y != 0 {
				t.Fatalf("the pages should share the same canvas origin, got %+v", origin)
			}
		}
	}
}
