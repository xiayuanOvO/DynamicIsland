package nowplaying

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestIsAppleMusicRunningDoesNotPanic(t *testing.T) {
	// Smoke test that NSWorkspace.runningApplications via purego/objc succeeds.
	_ = isAppleMusicRunning()
}

func TestWatchStreamsUpdatesAndCleansUpProcess(t *testing.T) {
	origCheck := isMusicRunning
	isMusicRunning = func() bool { return false }
	t.Cleanup(func() { isMusicRunning = origCheck })

	dir := t.TempDir()
	countFile := filepath.Join(dir, "invocations")
	adapter := filepath.Join(dir, "media-control")
	script := `#!/bin/sh
echo 1 >> "` + countFile + `"
printf '%s\n' '{"type":"data","diff":false,"payload":{}}'
printf '%s\n' '{"type":"data","diff":false,"payload":{"bundleIdentifier":"com.spotify.client","contentItemIdentifier":"sp-1","title":"灯塔","artist":"林海风","album":"远方","playing":true,"duration":240,"elapsedTime":12.5,"playbackRate":1,"timestamp":"2026-11-25T02:50:01Z"}}'
sleep 30
`
	if err := os.WriteFile(adapter, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tracks := make(chan Track, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Watch(ctx, adapter, func(track Track) {
			tracks <- track
		})
	}()

	select {
	case first := <-tracks:
		if first.Title != "" || first.Playing {
			t.Fatalf("expected initial idle track, got %+v", first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial idle stream event")
	}

	select {
	case second := <-tracks:
		wantSampled, _ := time.Parse(time.RFC3339, "2026-11-25T02:50:01Z")
		if second.Source != "com.spotify.client" || second.Title != "灯塔" || !second.Playing ||
			second.Position != 12.5 || second.Duration != 240 || !second.Sampled.Equal(wantSampled) {
			t.Fatalf("unexpected streamed track: %+v", second)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for playing stream event")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not terminate promptly after context cancellation")
	}

	raw, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Fields(string(raw)); len(lines) != 1 {
		t.Fatalf("expected exactly 1 media-control process invocation, got %d", len(lines))
	}
}

func TestWatchRestartsOnUnexpectedExit(t *testing.T) {
	origCheck := isMusicRunning
	isMusicRunning = func() bool { return false }
	t.Cleanup(func() { isMusicRunning = origCheck })

	dir := t.TempDir()
	countFile := filepath.Join(dir, "invocations")
	adapter := filepath.Join(dir, "media-control")
	script := `#!/bin/sh
echo 1 >> "` + countFile + `"
n=$(wc -l < "` + countFile + `" | tr -d ' ')
if [ "$n" = "1" ]; then
  printf '%s\n' '{"type":"data","diff":false,"payload":{"bundleIdentifier":"com.spotify.client","title":"第一首","playing":true,"duration":180,"elapsedTime":1}}'
  exit 1
fi
printf '%s\n' '{"type":"data","diff":false,"payload":{"bundleIdentifier":"com.spotify.client","title":"第二首","playing":true,"duration":200,"elapsedTime":5}}'
sleep 30
`
	if err := os.WriteFile(adapter, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tracks := make(chan Track, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Watch(ctx, adapter, func(track Track) {
			tracks <- track
		})
	}()

	for _, want := range []string{"第一首", "第二首"} {
		select {
		case got := <-tracks:
			if got.Title != want {
				t.Fatalf("got title %q, want %q", got.Title, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not exit after cancel")
	}
}
