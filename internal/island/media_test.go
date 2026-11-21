package island

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

func TestArtworkSurvivesResumingAppleMusic(t *testing.T) {
	cacheDir := t.TempDir()
	paused := nowplaying.Track{
		Source: "applescript:com.apple.Music", ID: "persistent-id",
		Title: "示例曲目", Artist: "示例艺人", Album: "示例专辑",
	}
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 180, G: 120, B: 60, A: 255})
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	paused.Artwork = data.Bytes()
	cover, tint, hash, _ := resolveArtwork(paused, nowplaying.Track{}, [32]byte{}, cacheDir)
	if cover == nil || tint != ui.RGB(180, 120, 60) {
		t.Fatalf("initial artwork: cover=%v, tint=%v", cover != nil, tint)
	}

	playing := paused
	playing.Source, playing.ID, playing.Playing, playing.Artwork = "com.apple.Music", "content-id", true, nil
	if cover, _, _, keep := resolveArtwork(playing, paused, hash, cacheDir); cover != nil || !keep {
		t.Fatal("resuming the same song should keep the displayed cover")
	}

	playing.Artwork = []byte("incomplete response")
	if cover, color, restoredHash, _ := resolveArtwork(playing, nowplaying.Track{}, [32]byte{}, cacheDir); cover == nil || color != tint || restoredHash != hash {
		t.Fatal("missing or invalid artwork should fall back to the cached cover after restart")
	}

	other := playing
	other.ID, other.Album, other.Artwork = "other-content-id", "另一张专辑", nil
	if cover, _, _, keep := resolveArtwork(other, playing, hash, cacheDir); cover != nil || keep {
		t.Fatal("switching albums should not reuse the previous cover")
	}
}

func TestArtworkCacheEvictsOldestCover(t *testing.T) {
	dir := t.TempDir()
	first := artworkCachePath(dir, [4]string{"player", "first"})
	second := artworkCachePath(dir, [4]string{"player", "second"})
	for i, path := range []string{first, second} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(path, maxArtworkCacheSize/2); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}

	key := [4]string{"player", "third"}
	saveArtwork(dir, key, []byte("new cover"))
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("oldest cover should have been evicted: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("more recent cover should still be cached: %v", err)
	}
	if data, err := os.ReadFile(artworkCachePath(dir, key)); err != nil || string(data) != "new cover" {
		t.Fatalf("new cache entry: %q, %v", data, err)
	}
}

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
