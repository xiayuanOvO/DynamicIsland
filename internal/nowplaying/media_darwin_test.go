package nowplaying

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadKeepsSystemTimelineWithArtworkFallback(t *testing.T) {
	for _, tt := range []struct {
		name, source, artwork string
		wantLoader            bool
	}{
		{"Music with no artwork", "com.apple.Music", "", true},
		{"Music with artwork", "com.apple.Music", "Y292ZXI=", false},
		{"other app with no artwork", "com.example.player", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"bundleIdentifier": tt.source, "contentItemIdentifier": "12345",
				"title": "夜行列车", "artist": "林海风", "album": "远方", "artworkData": tt.artwork,
				"playing": true, "duration": 240, "elapsedTimeNow": 112,
			})
			if err != nil {
				t.Fatal(err)
			}
			adapter := filepath.Join(t.TempDir(), "media-control")
			script := "#!/bin/sh\ncat <<'PAYLOAD'\n" + string(payload) + "\nPAYLOAD\n"
			if err := os.WriteFile(adapter, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			track, err := Read(t.Context(), adapter, true)
			if err != nil {
				t.Fatal(err)
			}
			if track.Source != tt.source || track.ID != "12345" || track.Position != 112 || track.Duration != 240 || !track.CanSeek() {
				t.Fatalf("system metadata was replaced: %+v", track)
			}
			if (track.ArtworkLoader != nil) != tt.wantLoader || (len(track.Artwork) > 0) != (tt.artwork != "") {
				t.Errorf("artwork bytes = %d, fallback available = %v", len(track.Artwork), track.ArtworkLoader != nil)
			}
		})
	}
}
