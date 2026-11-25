package nowplaying

import (
	"context"
	"math"
	"strings"
	"time"
)

type Track struct {
	Source, ID, Title, Artist, Album string
	Artwork                          []byte
	Position, Duration               float64
	Rate                             float64
	Sampled                          time.Time
	Playing                          bool
	Previous, Next                   bool
	Toggle, Seek                     bool
	EstimatedPosition                bool
	ArtworkLoader                    func(context.Context) []byte
}

func (t Track) SameSong(other Track) bool {
	if t.Source != other.Source {
		return false
	}
	if t.ID != "" && other.ID != "" {
		return t.ID == other.ID
	}
	return t.Title == other.Title && t.Artist == other.Artist && t.Album == other.Album
}

func (t Track) ArtworkKey() [4]string {
	// MediaRemote and AppleScript report different IDs for the same Music track.
	return [4]string{strings.TrimPrefix(t.Source, "applescript:"), t.Title, t.Artist, t.Album}
}

func (t Track) CanSeek() bool { return t.Seek && t.Duration > 0 }

func (t Track) IsNetEase() bool { return strings.EqualFold(t.Source, "cloudmusic.exe") }

func (t Track) HasPosition() bool { return t.Duration > 0 || t.Position > 0 || t.EstimatedPosition }

// EstimateNetEasePosition keeps a clock for sessions that only report the song and play state.
func (t *Track) EstimateNetEasePosition(previous Track) {
	if !t.IsNetEase() || t.Title == "" || t.HasPosition() {
		return
	}

	if t.SameSong(previous) && previous.HasPosition() {
		t.Position = previous.Elapsed(t.Sampled)
		t.Duration = previous.Duration
	}
	t.EstimatedPosition = true
	if t.Rate <= 0 {
		t.Rate = 1
	}
}

func (t Track) Elapsed(now time.Time) float64 {
	position := t.Position
	if t.Playing {
		position += max(0, now.Sub(t.Sampled).Seconds()) * t.Rate
	}
	if t.Duration > 0 {
		return math.Min(t.Duration, math.Max(0, position))
	}
	return math.Max(0, position)
}

type Action string

const (
	Previous Action = "previous-track"
	Next     Action = "next-track"
	Toggle   Action = "toggle-play-pause"
	Seek     Action = "seek"
)

type Command struct {
	Action   Action
	Source   string
	Position float64
}

func sleepWakeDetected(prev, now time.Time, expected time.Duration) bool {
	if prev.IsZero() {
		return false
	}
	const margin = 15 * time.Second
	return now.Round(0).Sub(prev.Round(0)) > expected+margin || now.Sub(prev) > expected+margin
}
