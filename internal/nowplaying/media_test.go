package nowplaying

import (
	"testing"
	"time"
)

func TestTimeline(t *testing.T) {
	at := time.Unix(100, 0)
	track := Track{Position: 37, Duration: 42, Playing: true, Seek: true, Rate: 1.5, Sampled: at}
	if got := track.Elapsed(at.Add(2 * time.Second)); got != 40 {
		t.Errorf("playing position = %v, want 40", got)
	}
	if got := track.Elapsed(at.Add(time.Minute)); got != 42 {
		t.Errorf("position past end = %v, want 42", got)
	}
	track.Playing = false
	if got := track.Elapsed(at.Add(time.Minute)); got != 37 {
		t.Errorf("paused position = %v, want 37", got)
	}
	track.Duration = 0
	if track.CanSeek() {
		t.Error("media without duration cannot seek")
	}
}

func TestSameSong(t *testing.T) {
	track := Track{Source: "music", ID: "a", Title: "Song", Artist: "Artist", Album: "Album"}
	if !track.SameSong(track) {
		t.Fatal("same track ID was not recognized")
	}
	sameTitle := track
	sameTitle.ID = "b"
	if track.SameSong(sameTitle) {
		t.Fatal("distinct tracks shared the same identity")
	}
	sameTitle.ID = ""
	if !track.SameSong(sameTitle) {
		t.Fatal("metadata fallback failed when the ID was temporarily absent")
	}
	sameTitle.Album = "Another album"
	if track.SameSong(sameTitle) {
		t.Fatal("same title and artist on different albums should be distinct")
	}
}
