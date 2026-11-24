package island

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"dynamicisland/internal/lyrics"
	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

const (
	lyricOffsetStep = 0.5
	lyricOffsetMax  = 30.0
)

func songForTrack(track nowplaying.Track) lyrics.Song {
	return lyrics.Song{
		Title:    strings.TrimSpace(track.Title),
		Artist:   strings.TrimSpace(track.Artist),
		Album:    strings.TrimSpace(track.Album),
		Duration: math.Round(track.Duration),
	}
}

func lyricOffsetKey(song lyrics.Song) string {
	if song.Title == "" {
		return ""
	}
	return song.Title + "\x00" + song.Artist + "\x00" + song.Album
}

func (a *island) lyricOffsetsPath() string {
	return filepath.Join(filepath.Dir(a.statePath), "lyric-offsets.json")
}

func (a *island) loadLyricOffsets() {
	if a.statePath == "" {
		return
	}
	data, err := os.ReadFile(a.lyricOffsetsPath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		log.Printf("load lyric offsets: %v", err)
		return
	}
	var offsets map[string]float64
	if err := json.Unmarshal(data, &offsets); err != nil {
		log.Printf("parse lyric offsets: %v", err)
		return
	}
	a.lyrics.offsets = offsets
}

func (a *island) lyricOffset(song lyrics.Song) float64 {
	key := lyricOffsetKey(song)
	if key == "" || a.lyrics.offsets == nil {
		return 0
	}
	return a.lyrics.offsets[key]
}

func (a *island) setLyricOffset(song lyrics.Song, value float64) {
	key := lyricOffsetKey(song)
	if key == "" {
		return
	}
	// Round to nearest 0.1s to avoid floating-point drift on repeated ±0.5s clicks.
	value = math.Round(max(-lyricOffsetMax, min(lyricOffsetMax, value))*10) / 10
	if a.lyrics.offsets == nil {
		a.lyrics.offsets = make(map[string]float64)
	}
	if value == 0 {
		delete(a.lyrics.offsets, key)
	} else {
		a.lyrics.offsets[key] = value
	}
	if a.statePath != "" {
		if err := saveJSON(a.lyricOffsetsPath(), a.lyrics.offsets); err != nil {
			log.Printf("save lyric offsets: %v", err)
		}
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.settingsWindow != nil {
		a.settingsWindow.Invalidate()
	}
}

func formatLyricOffset(offset float64) string {
	if offset > 0 {
		return fmt.Sprintf("+%.1fs", offset)
	}
	if offset < 0 {
		return fmt.Sprintf("%.1fs", offset)
	}
	return "0.0s"
}

func (a *island) lyricOffsetSettings(c *ui.Context) {
	song := songForTrack(a.media.track)
	hasSong := song.Title != ""
	offset := a.lyricOffset(song)
	settingsRow(c.Key("lyric-offset-settings")).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "歌词偏移").FontSize(13).TextColor(c.Theme().Text)
			if hasSong {
				ui.Text(c, song.Title).SingleLine().Ellipsis("…").FontSize(11).TextColor(c.Theme().TextMuted)
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.Button(c.Key("lyric-offset-minus"), "-0.5s").Height(32).FontSize(12).
				Disabled(!hasSong || offset <= -lyricOffsetMax).Label("歌词延后 0.5 秒").Clicked() {
				a.setLyricOffset(song, offset-lyricOffsetStep)
			}
			label := formatLyricOffset(offset)
			if offset != 0 && hasSong {
				if ui.Button(c.Key("lyric-offset-reset"), label).Width(56).Height(32).FontSize(12).
					Label("重置歌词偏移 " + label).Clicked() {
					a.setLyricOffset(song, 0)
				}
			} else {
				ui.Text(c, label).Width(56).FontSize(12).FontFeatures("tnum").TextAlign(ui.Center).TextColor(c.Theme().TextMuted)
			}
			if ui.Button(c.Key("lyric-offset-plus"), "+0.5s").Height(32).FontSize(12).
				Disabled(!hasSong || offset >= lyricOffsetMax).Label("歌词提前 0.5 秒").Clicked() {
				a.setLyricOffset(song, offset+lyricOffsetStep)
			}
		})
	})
}
