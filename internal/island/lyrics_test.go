package island

import (
	"bytes"
	"strings"
	"testing"

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
