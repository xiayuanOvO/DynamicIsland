package main

import (
	"path/filepath"
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestSettingsSavesMusicCardStyle(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	settings, err := loadSettings(a.settingsPath())
	if err != nil || settings.MediaCardStyle != mediaCardDefault {
		t.Fatalf("expected default music card style: %+v, %v", settings, err)
	}
	view := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
	if cover, found := view.Find("专辑封面"); !found || cover.W != 40 {
		t.Fatalf("settings should start with the default card preview: %+v", cover)
	}
	for _, choice := range []struct {
		label      string
		style      int
		coverWidth float32
	}{
		{"大封面", mediaCardLargeCover, largeMediaCoverSize},
		{"默认风格", mediaCardDefault, 40},
	} {
		if err := view.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		if cover, found := view.Find("专辑封面"); !found || cover.W != choice.coverWidth {
			t.Fatalf("preview did not immediately switch to %s: %+v", choice.label, cover)
		}
		view.Advance(time.Second)
		saved, err := loadSettings(a.settingsPath())
		if err != nil || saved.MediaCardStyle != choice.style {
			t.Fatalf("saved music card after choosing %s: %+v, %v", choice.label, saved, err)
		}
	}
}

func TestMusicCardPreviewIsReadOnly(t *testing.T) {
	a := island{}
	a.media.track = nowplaying.Track{Title: "当前曲目", Playing: true, Toggle: true, Next: true, Duration: 240, Seek: true}
	a.media.reading = true
	view := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
	pause, found := view.Find("暂停")
	if !found {
		t.Fatal("preview has no playback controls")
	}
	view.ClickAt(pause.X+pause.W/2, pause.Y+pause.H/2)
	if a.media.pending.Action != "" || a.media.scrubbing {
		t.Fatalf("preview interacted with playback: %+v", a.media.pending)
	}
}

func TestMusicCardStylesPlaceCoverAndControls(t *testing.T) {
	for _, style := range []int{mediaCardDefault, mediaCardLargeCover} {
		a := island{files: &shelf.Shelf{}, expanded: true, settings: appSettings{MediaCardStyle: style}}
		track := nowplaying.Track{
			Title: "曲目名称", Artist: "音乐人", Duration: 246, Position: 41,
			Playing: true, Seek: true, Previous: true, Next: true, Toggle: true,
		}
		a.media.track = track
		height, _ := a.panelHeights()
		cardHeight := a.settings.cardHeight()
		if height != compactHeight+10+20+cardHeight {
			t.Fatalf("panel does not account for the selected card: %d, card %d", height, cardHeight)
		}
		view := ui.NewTester(a.mediaCard, panelWidth-46, cardHeight)
		cover, hasCover := view.Find("专辑封面")
		title, hasTitle := view.Find(track.Title)
		seek, hasSeek := view.Find("播放进度")
		pause, hasPause := view.Find("暂停")
		if !hasCover || !hasTitle || !hasSeek || !hasPause {
			t.Fatalf("style %d is missing a cover, metadata, seek or pause", style)
		}
		if style == mediaCardLargeCover {
			if cover.W < 110 || cover.W != cover.H || cover.X+cover.W >= min(title.X, seek.X, pause.X) ||
				seek.Y <= title.Y+title.H || pause.Y <= seek.Y+seek.H || pause.Y+pause.H > float32(cardHeight) {
				t.Fatalf("expected a large cover beside metadata, progress and controls: %+v, %+v, %+v, %+v", cover, title, seek, pause)
			}
		} else if cover.W != 40 || seek.Y < cover.Y+cover.H || pause.Y >= seek.Y {
			t.Fatalf("default card layout changed: %+v, %+v, %+v", cover, seek, pause)
		}
	}
}
