package island

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestLyricOverlayWrapsOnlyCurrentLine(t *testing.T) {
	long := strings.Repeat("远处的列车经过海岸和街道，", 5)
	longer := strings.Repeat("风把歌声带到安静的花园，", 6)
	short := "夜色渐渐散去"
	tests := []struct {
		name               string
		from, to           string
		fromWraps, toWraps bool
	}{
		{"short to long", short, long, false, true},
		{"long to short", long, short, true, false},
		{"long to long", long, longer, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := lyricOverlay{rows: [3]string{longer + "前", test.from, test.to}, side: dockRight}
			view := ui.NewTester(o.view, 436, lyricOverlayHeight)
			bounds := func(line string) ui.Rect {
				t.Helper()
				r, ok := view.Find(line)
				if !ok {
					t.Fatalf("lyric is missing: %q", line)
				}
				return r
			}
			checkHeight := func(r ui.Rect, wrapped bool) {
				t.Helper()
				if wrapped && r.H != 52 || !wrapped && r.H >= 30 {
					t.Fatalf("unexpected text height %v (two lines: %v)", r.H, wrapped)
				}
			}
			beforeFrom, beforeTo := bounds(test.from), bounds(test.to)
			checkHeight(bounds(o.rows[0]), false)
			checkHeight(beforeFrom, test.fromWraps)
			checkHeight(beforeTo, false)

			before := append([]byte(nil), view.Image().Pix...)
			o.outgoing, o.transitionAt = o.rows[0], view.Now()
			o.rows = [3]string{test.from, test.to, longer + "后"}
			view.Frame()
			if !bytes.Equal(before, view.Image().Pix) {
				t.Fatal("transition should start at the previous layout")
			}
			view.Advance(lyricTransitionDuration / 3)
			view.Frame()
			if bounds(test.from).Y >= beforeFrom.Y || bounds(test.to).Y >= beforeTo.Y {
				t.Fatal("both lines should move up during the transition")
			}
			view.Advance(lyricTransitionDuration)
			view.Frame()
			checkHeight(bounds(test.from), false)
			checkHeight(bounds(test.to), test.toWraps)
			checkHeight(bounds(o.rows[2]), false)
		})
	}
}

func TestLyricOverlayTransitionStyles(t *testing.T) {
	tests := []struct {
		name         string
		style        int
		reduceMotion bool
		animates     bool
	}{
		{"fade", lyricTransitionFade, false, true},
		{"zoom", lyricTransitionZoom, false, true},
		{"no animation", lyricTransitionNone, false, false},
		{"reduced motion with slide", lyricTransitionSlide, true, false},
		{"reduced motion with fade", lyricTransitionFade, true, false},
		{"reduced motion with zoom", lyricTransitionZoom, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := strings.Repeat("风把歌声带过安静的海面，", 5)
			o := lyricOverlay{rows: [3]string{"落日照在窗上", current, "天色慢慢暗下来"}, side: dockRight, transition: test.style}
			view := ui.NewTesterWith(o.view, 436, lyricOverlayHeight, ui.TesterOptions{
				Preferences: ui.Preferences{ReduceMotion: test.reduceMotion},
			})
			settled := view.Image()
			o.outgoing, o.transitionAt = "上一段歌声", view.Now()
			view.Frame()
			start := view.Image()
			if changed := !bytes.Equal(settled.Pix, start.Pix); changed != test.animates {
				t.Fatalf("initial frame changed = %t, want %t", changed, test.animates)
			}
			if !view.HasText(current) {
				t.Fatal("the new lyric should be present from the first frame")
			}
			view.Advance(lyricTransitionDuration / 3)
			if changed := !bytes.Equal(start.Pix, view.Image().Pix); changed != test.animates {
				t.Fatalf("intermediate frame changed = %t, want %t", changed, test.animates)
			}
			view.Advance(lyricTransitionDuration)
			if !bytes.Equal(settled.Pix, view.Image().Pix) {
				t.Fatal("the lyric did not settle at its normal appearance")
			}
		})
	}
}

func TestLyricOverlayScrollsOverflowingSecondLine(t *testing.T) {
	twoLines := "天亮以前路灯把影子拉得很长，等第一班车穿过安静的街道"
	overflow := strings.Repeat("🎶远处有风，海面正在泛起微光。", 5) + "此刻天亮"
	tests := []struct {
		name, line   string
		reduceMotion bool
		scrolls      bool
	}{
		{"fits on two lines", twoLines, false, false},
		{"overflows two lines", overflow, false, true},
		{"reduced motion", overflow, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := lyricOverlay{rows: [3]string{"风从海上吹来", test.line, "列车到了"}, side: dockRight}
			view := ui.NewTesterWith(o.view, 436, lyricOverlayHeight, ui.TesterOptions{
				Preferences: ui.Preferences{ReduceMotion: test.reduceMotion},
			})
			o.currentSince = view.Now()
			o.lineEndsAt = view.Now().Add(8 * time.Second)
			view.Frame()
			before := view.Image()
			view.Advance(lyricScrollDelay / 2)
			view.Frame()
			if !bytes.Equal(before.Pix, view.Image().Pix) {
				t.Fatal("lyrics moved during the initial reading pause")
			}

			view.Advance(time.Second)
			view.Frame()
			moving := view.Image()
			if changed := !bytes.Equal(before.Pix, moving.Pix); changed != test.scrolls {
				t.Fatalf("horizontal scrolling = %v, want %v", changed, test.scrolls)
			}
			if !test.scrolls {
				return
			}
			firstLine := (lyricOverlayHeight/2 - 8) * before.Stride
			if !bytes.Equal(before.Pix[:firstLine], moving.Pix[:firstLine]) {
				t.Fatal("the first line moved with the second")
			}

			view.Advance(o.lineEndsAt.Sub(view.Now()) - lyricScrollHold/2)
			view.Frame()
			end := view.Image()
			view.Advance(lyricScrollHold / 4)
			view.Frame()
			if !bytes.Equal(end.Pix, view.Image().Pix) {
				t.Fatal("scrolling should already be stopped before the next lyric")
			}

			o.outgoing, o.previousOffset, o.transitionAt = o.rows[0], o.scroll.offset(view.Now()), view.Now()
			o.currentSince = view.Now()
			o.lineEndsAt = view.Now().Add(4 * time.Second)
			o.scroll = lyricScroll{}
			o.rows = [3]string{test.line, o.rows[2], "天边逐渐亮起来"}
			view.Frame()
			if !bytes.Equal(end.Pix, view.Image().Pix) {
				t.Fatal("the previous lyric should keep its horizontal position when the upward transition starts")
			}
		})
	}
}

func TestLyricScrollAdjustsToPlaybackDeadline(t *testing.T) {
	now := time.Unix(1000, 0)
	const overflow = 420
	longEnd, shortEnd := now.Add(8*time.Second), now.Add(3*time.Second)
	var normal, faster lyricScroll
	normal.retarget(now, now, longEnd, overflow)
	faster.retarget(now, now, shortEnd, overflow)
	sample := now.Add(time.Second)
	if normal.offset(sample) <= 0 || faster.offset(sample) <= normal.offset(sample) {
		t.Fatal("a shorter line duration should produce faster scrolling")
	}
	for _, scroll := range []lyricScroll{normal, faster} {
		if scroll.offset(scroll.deadline.Add(-lyricScrollHold)) != overflow {
			t.Fatal("the full tail should be visible before the deadline")
		}
	}

	distance := normal.offset(sample)
	normal.retarget(sample, now, shortEnd, overflow)
	if normal.offset(sample) != distance || normal.offset(shortEnd) != overflow {
		t.Fatal("an earlier deadline should speed up scrolling from the current position")
	}
	nextSample := sample.Add(time.Second)
	distance = normal.offset(nextSample)
	normal.retarget(nextSample, now, longEnd, overflow)
	if normal.offset(nextSample) != distance || normal.finish.Before(longEnd.Add(-lyricScrollHold)) {
		t.Fatal("a later deadline should slow scrolling from the current position")
	}

	var tight lyricScroll
	tightEnd := now.Add(1600 * time.Millisecond)
	tight.retarget(now, now, tightEnd, overflow)
	if tight.start.Sub(now) >= lyricScrollDelay || !tight.finish.Before(tightEnd) || tight.offset(tightEnd) != overflow {
		t.Fatal("shorter reading pauses should let the tail appear before a tight deadline")
	}

	var veryShort lyricScroll
	veryShort.retarget(now, now, now.Add(time.Second), overflow)
	if veryShort.start != now || veryShort.offset(now.Add(time.Second)) != lyricScrollMaxRate {
		t.Fatal("when time is tight, scrolling should start immediately at the maximum speed")
	}
}
