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
			o.outgoing, o.shiftedAt = o.rows[0], view.Now()
			o.rows = [3]string{test.from, test.to, longer + "后"}
			view.Frame()
			if !bytes.Equal(before, view.Image().Pix) {
				t.Fatal("transition should start at the previous layout")
			}
			view.Advance(lyricTransition / 3)
			view.Frame()
			if bounds(test.from).Y >= beforeFrom.Y || bounds(test.to).Y >= beforeTo.Y {
				t.Fatal("both lines should move up during the transition")
			}
			view.Advance(lyricTransition)
			view.Frame()
			checkHeight(bounds(test.from), false)
			checkHeight(bounds(test.to), test.toWraps)
			checkHeight(bounds(o.rows[2]), false)
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

			view.Advance(time.Minute)
			view.Frame()
			end := view.Image()
			view.Advance(time.Second)
			view.Frame()
			if !bytes.Equal(end.Pix, view.Image().Pix) {
				t.Fatal("scrolling should stop at the end of the lyric")
			}

			o.outgoing, o.previousSince, o.shiftedAt = o.rows[0], o.currentSince, view.Now()
			o.currentSince = view.Now()
			o.rows = [3]string{test.line, o.rows[2], "天边逐渐亮起来"}
			view.Frame()
			if !bytes.Equal(end.Pix, view.Image().Pix) {
				t.Fatal("the previous lyric should keep its horizontal position when the upward transition starts")
			}
		})
	}
}
