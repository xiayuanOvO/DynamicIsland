package main

import (
	"testing"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestCompactHeaderShowsWaveOnlyDuringPlayback(t *testing.T) {
	for _, test := range []struct {
		name     string
		playing  bool
		expanded bool
		wantWave bool
	}{
		{name: "playing", playing: true, wantWave: true},
		{name: "paused"},
		{name: "expanded", playing: true, expanded: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := island{files: &shelf.Shelf{}, expanded: test.expanded}
			a.media.track = nowplaying.Track{Title: "曲目", Playing: test.playing}
			var open float32
			if test.expanded {
				open = 1
			}
			view := ui.NewTester(func(c *ui.Context) { a.header(c, open, open) }, compactWidth, compactHeight)
			if _, branded := view.Find("灵动岛"); branded {
				t.Fatal("header still displays the app name")
			}
			if _, hasWave := view.Find("播放律动"); hasWave != test.wantWave {
				t.Fatalf("wave visible = %v, want %v", hasWave, test.wantWave)
			}
		})
	}
}
