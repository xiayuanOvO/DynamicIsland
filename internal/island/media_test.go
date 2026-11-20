package island

import (
	"bytes"
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

func TestMediaCardShowsProgressWithCatalogDuration(t *testing.T) {
	for _, style := range []struct {
		name string
		kind int
	}{
		{"spectrum", mediaCardDefault},
		{"large cover", mediaCardLargeCover},
	} {
		t.Run(style.name, func(t *testing.T) {
			settings := appSettings{MediaCardStyle: style.kind}
			media := islandMedia{
				track:           nowplaying.Track{Source: "cloudmusic.exe", Title: "示例曲目", Rate: 1, Playing: true, Seek: true},
				catalogDuration: 120,
			}
			view := ui.NewTester(func(c *ui.Context) {
				settings.renderMediaCard(c, &media, true, func(action nowplaying.Action, position float64) {
					t.Errorf("unexpected control: %s at %v", action, position)
				})
			}, 360, settings.cardHeight())
			media.track.Sampled = view.Now().Add(-30 * time.Second)
			view.Frame()
			elapsed, ok := view.Find("0:00")
			if !ok || !view.HasText("2:00") {
				t.Fatal("the catalog duration should be visible before an elapsed time is available")
			}
			rowEnd := int(elapsed.Y) * view.Image().Stride
			unplayed := append([]byte(nil), view.Image().Pix[:rowEnd]...)

			media.track.EstimatedPosition = true
			view.Frame()
			if !view.HasText("0:30") || bytes.Equal(unplayed, view.Image().Pix[:rowEnd]) {
				t.Fatal("the estimated position should color the played part of the track")
			}
			if _, seek := view.Find("播放进度"); seek {
				t.Fatal("a duration from the catalog should not enable seeking")
			}
			fromCatalog := append([]byte(nil), view.Image().Pix...)

			media.track.Duration = 120
			media.track.Seek = false
			view.Frame()
			if !bytes.Equal(fromCatalog, view.Image().Pix) {
				t.Fatal("catalog and native duration should render the same progress")
			}
		})
	}
}
