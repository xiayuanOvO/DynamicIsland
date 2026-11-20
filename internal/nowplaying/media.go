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

func (t Track) CanSeek() bool { return t.Seek && t.Duration > 0 }

// EstimateNetEasePosition follows playback from a song change observed in the Windows session.
func (t *Track) EstimateNetEasePosition(previous Track) {
	gap := t.Sampled.Sub(previous.Sampled)
	if t.Duration > 0 || t.Position != 0 || !strings.EqualFold(t.Source, "cloudmusic.exe") || t.Source != previous.Source ||
		t.Title == "" || previous.Title == "" || gap <= 0 || gap > 5*time.Second {
		return
	}

	// A change can have happened anywhere between the two samples.
	midpoint := previous.Sampled.Add(gap / 2)
	if t.SameSong(previous) {
		if !previous.EstimatedPosition {
			return
		}
		t.Position = previous.Elapsed(midpoint)
	}
	if t.Playing {
		t.Position += t.Sampled.Sub(midpoint).Seconds() * t.Rate
	}
	t.EstimatedPosition = true
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
