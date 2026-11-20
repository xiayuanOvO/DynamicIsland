package nowplaying

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
