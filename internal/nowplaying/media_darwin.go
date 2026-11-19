package nowplaying

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

func Adapter() string {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return ""
	}
	adapter, _ := exec.LookPath(filepath.Join(resources, "media-control", "bin", "media-control"))
	return adapter
}

//go:embed music_darwin.js
var musicScript string

//go:embed artwork_darwin.applescript
var musicArtworkScript string

const musicScriptSource = "applescript:com.apple.Music"

func Read(ctx context.Context, adapter string, withArtwork bool) (Track, error) {
	track, hasTimeline, err := readSystemMedia(ctx, adapter, withArtwork)
	isMusic := track.Source == "com.apple.Music"
	if track.Title != "" && track.Playing && (!isMusic || hasTimeline) {
		if isMusic && len(track.Artwork) == 0 {
			track.ArtworkLoader = musicArtworkLoader("", track)
		}
		return track, nil
	}

	data, musicErr := runMusicScript(ctx)
	if musicErr == nil && len(data) > 0 {
		var music Track
		if json.Unmarshal(data, &music) == nil && music.Title != "" {
			music.Source = musicScriptSource
			music.Sampled = time.Now()
			music.Rate = 1
			music.Previous, music.Next, music.Toggle, music.Seek = true, true, true, true
			if isMusic && track.Title == music.Title && track.Artist == music.Artist {
				music.Artwork = track.Artwork
			}
			if music.ID != "" {
				music.ArtworkLoader = musicArtworkLoader(music.ID, music)
			}
			return music, nil
		}
	}
	return track, err
}

func musicArtworkLoader(persistentID string, track Track) func(context.Context) []byte {
	args := []string{"-s", "s", "-", persistentID, track.Title, track.Artist, track.Album}
	return func(ctx context.Context) []byte {
		cmd := exec.CommandContext(ctx, "/usr/bin/osascript", args...)
		cmd.Stdin = strings.NewReader(musicArtworkScript)
		data, err := cmd.Output()
		raw, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "«data ")
		if err != nil || !ok || len(raw) <= 6 || len(raw) > (16<<20)+6 || !strings.HasSuffix(raw, "»") {
			return nil
		}
		artwork, _ := hex.DecodeString(raw[4 : len(raw)-len("»")])
		return artwork
	}
}

func readSystemMedia(ctx context.Context, adapter string, withArtwork bool) (Track, bool, error) {
	args := []string{"get", "--now"}
	if !withArtwork {
		args = append(args, "--no-artwork")
	}
	data, err := exec.CommandContext(ctx, adapter, args...).Output()
	if err != nil {
		return Track{}, false, err
	}
	var payload struct {
		BundleIdentifier, ContentItemIdentifier, Title, Artist, Album string
		ArtworkData                                                   []byte
		Duration                                                      float64
		ElapsedTime                                                   *float64
		ElapsedTimeNow, PlaybackRate                                  *float64
		Playing                                                       bool
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Track{}, false, err
	}
	elapsed := payload.ElapsedTimeNow
	if elapsed == nil {
		elapsed = payload.ElapsedTime
	}
	var position float64
	if elapsed != nil {
		position = *elapsed
	}
	rate := 1.0
	if payload.PlaybackRate != nil {
		rate = *payload.PlaybackRate
	}
	return Track{
		Source: payload.BundleIdentifier, ID: payload.ContentItemIdentifier,
		Title: payload.Title, Artist: payload.Artist, Album: payload.Album,
		Artwork: payload.ArtworkData, Playing: payload.Playing, Position: position,
		Duration: payload.Duration, Rate: rate, Sampled: time.Now(),
		Previous: true, Next: true, Toggle: true, Seek: true,
	}, elapsed != nil && payload.Duration > 0, nil
}

func Control(ctx context.Context, adapter string, command Command) error {
	if command.Source == musicScriptSource {
		_, err := runMusicScript(ctx, string(command.Action), strconv.FormatFloat(command.Position, 'f', 3, 64))
		return err
	}
	args := []string{string(command.Action)}
	if command.Action == Seek {
		args = append(args, "--micros", strconv.FormatInt(int64(command.Position*1e6), 10))
	}
	return exec.CommandContext(ctx, adapter, args...).Run()
}

func runMusicScript(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", append([]string{"-l", "JavaScript", "-"}, args...)...)
	cmd.Stdin = strings.NewReader(musicScript)
	return cmd.Output()
}
