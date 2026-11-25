package nowplaying

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dynamicisland/internal/lyrics"
)

func TestReadWindowsArtwork(t *testing.T) {
	var artwork bytes.Buffer
	if err := png.Encode(&artwork, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(path, artwork.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	// Use the same WinRT stream projection as the media session without depending on a player.
	setup, _, ok := strings.Cut(sessionScript, "\n$manager =")
	if !ok {
		t.Fatal("missing media session entry point")
	}
	script := setup + `
$fileType = [Windows.Storage.StorageFile, Windows.Storage, ContentType=WindowsRuntime]
$referenceType = [Windows.Storage.Streams.RandomAccessStreamReference, Windows.Storage.Streams, ContentType=WindowsRuntime]
$file = Wait-Result ($fileType::GetFileFromPathAsync($env:DYNAMICISLAND_TEST_ARTWORK_PATH)) $fileType
Read-Artwork ($referenceType::CreateFromFile($file))
`
	cmd := exec.CommandContext(t.Context(), Adapter(), "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	cmd.Env = append(os.Environ(), "DYNAMICISLAND_TEST_ARTWORK_PATH="+path)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("read artwork: %v", err)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil || !bytes.Equal(data, artwork.Bytes()) {
		t.Fatalf("artwork from WinRT stream: %d bytes, want %d (decode error: %v)", len(data), artwork.Len(), err)
	}
}

func TestDecodeWindowsTrackForLyrics(t *testing.T) {
	const appleMusic = "AppleInc.AppleMusicWin_nzyj5cx40ttqa!App"
	for _, tt := range []struct {
		name, source, artist, album, wantArtist, wantAlbum string
	}{
		{"combined fields", appleMusic, "林海风 — 远方", "", "林海风", "远方"},
		{"dash within album", appleMusic, "林海风 — 远方 — 第二辑", "", "林海风", "远方 — 第二辑"},
		{"separate album", appleMusic, "林海风 — 江雁", "远方", "林海风 — 江雁", "远方"},
		{"artist only", appleMusic, "林海风 - 江雁", "", "林海风 - 江雁", ""},
		{"missing artist", appleMusic, " — 远方", "", " — 远方", ""},
		{"missing album", appleMusic, "林海风 — ", "", "林海风 — ", ""},
		{"other player", "other.exe", "林海风 — 江雁", "", "林海风 — 江雁", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := fmt.Sprintf(`{"Source":%q,"Artist":%q,"Album":%q}`, tt.source, tt.artist, tt.album)
			track, err := decodeWindowsTrack([]byte(data))
			if err != nil {
				t.Fatal(err)
			}
			if track.Artist != tt.wantArtist || track.Album != tt.wantAlbum {
				t.Fatalf("artist and album = %q, %q, want %q, %q", track.Artist, track.Album, tt.wantArtist, tt.wantAlbum)
			}
		})
	}

	t.Run("local lyrics follow Apple Music position", func(t *testing.T) {
		data := []byte(`{"Source":"AppleInc.AppleMusicWin_nzyj5cx40ttqa!App","Title":"灯塔","Artist":"林海风 — 远方","Album":"","Position":41,"Duration":234,"Rate":1,"Playing":true}`)
		track, err := decodeWindowsTrack(data)
		if err != nil {
			t.Fatal(err)
		}
		track.Sampled = time.Unix(1000, 0)
		folder := t.TempDir()
		if err := os.WriteFile(filepath.Join(folder, "林海风 - 灯塔.lrc"), []byte("[00:40.00]灯亮了\n[00:44.00]天快亮了"), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // The fixture must work without an online lookup.
		song := lyrics.Song{Title: track.Title, Artist: track.Artist, Album: track.Album, Duration: track.Duration}
		lines, _ := lyrics.NewCache(folder).Load(ctx, folder, song)
		if text, _ := lines.Current(track.Elapsed(track.Sampled.Add(3 * time.Second))); text != "天快亮了" {
			t.Errorf("lyric after playing for 3 seconds = %q, want 天快亮了", text)
		}
	})
}

func TestEstimateNetEasePosition(t *testing.T) {
	time0 := time.Unix(1000, 0)
	var previous Track
	for _, sample := range []struct {
		name, title string
		second      int
		playing     bool
		position    float64
	}{
		{"already playing on startup", "灯塔", 0, true, 0},
		{"first zero position refresh", "灯塔", 2, true, 2},
		{"second zero position refresh", "灯塔", 4, true, 4},
		{"pause", "灯塔", 6, false, 6},
		{"stays paused", "灯塔", 8, false, 6},
		{"resume", "灯塔", 10, true, 6},
		{"gap in samples", "灯塔", 30, true, 26},
		{"next observed song", "远山", 32, true, 0},
	} {
		t.Run(sample.name, func(t *testing.T) {
			track := Track{
				Source: "cloudmusic.exe", Title: sample.title, Playing: sample.playing,
				Sampled: time0.Add(time.Duration(sample.second) * time.Second),
			}
			track.EstimateNetEasePosition(previous)
			if !track.EstimatedPosition || !track.HasPosition() || track.Position != sample.position {
				t.Errorf("position = %v (estimated: %v), want estimated position %v", track.Position, track.EstimatedPosition, sample.position)
			}
			previous = track
		})
	}

	for _, track := range []Track{
		{Source: "cloudmusic.exe", Duration: 120},
		{Source: "cloudmusic.exe", Position: 15},
		{Source: "other.exe"},
	} {
		previous.Source = track.Source
		track.Title = "海岸"
		track.Sampled = previous.Sampled.Add(time.Second)
		position := track.Position
		track.EstimateNetEasePosition(previous)
		if track.EstimatedPosition || track.Position != position {
			t.Errorf("unexpected estimate for %s with duration %v", track.Source, track.Duration)
		}
	}
}

func TestWindowsTrackEquivalent(t *testing.T) {
	t0 := time.Unix(1000, 0)
	base := Track{
		Source: "Spotify.exe", Title: "灯塔", Artist: "林海风",
		Playing: true, Position: 10, Duration: 200, Rate: 1, Sampled: t0,
	}
	next := base
	next.Sampled = t0.Add(250 * time.Millisecond)
	next.Position = 10.25
	if !windowsTrackEquivalent(base, next) {
		t.Fatal("normal playback progression should be treated as equivalent")
	}
	seeked := next
	seeked.Position = 30
	if windowsTrackEquivalent(base, seeked) {
		t.Fatal("a seek jump should not be treated as equivalent")
	}
	paused := next
	paused.Playing = false
	if windowsTrackEquivalent(base, paused) {
		t.Fatal("play/pause change should not be treated as equivalent")
	}
	if !windowsTrackEquivalent(Track{}, Track{}) {
		t.Fatal("two empty idle tracks should be equivalent")
	}
}
