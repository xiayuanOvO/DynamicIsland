package island

import (
	"context"
	"log"
	"math"
	"path/filepath"
	"strings"
	"time"

	"dynamicisland/internal/lyrics"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const lyricTransition = 240 * time.Millisecond

type lyricOverlay struct {
	win           *mygo.Window
	cache         map[lyrics.Song]lyrics.Lines
	diskCache     lyrics.Cache
	cacheVersion  uint64
	cacheBytes    int64
	cacheSizeErr  error
	cacheClearErr error
	loading       bool
	song          lyrics.Song
	index         int
	rows          [3]string
	outgoing      string
	shiftedAt     time.Time
	anchor        mygo.Rectangle
	side          dockSide
}

func (a *island) updateLyrics(c *ui.Context, frame islandFrame, width, height int) {
	overlay := &a.lyrics
	track := a.media.track
	var song lyrics.Song
	index := -1
	var rows [3]string
	var anchor mygo.Rectangle
	var side dockSide
	if !a.settings.HideLyrics && a.win.IsVisible() && track.Playing && track.Title != "" && track.Duration > 0 {
		song = lyrics.Song{
			Title: strings.TrimSpace(track.Title), Artist: strings.TrimSpace(track.Artist),
			Album: strings.TrimSpace(track.Album), Duration: math.Round(track.Duration),
		}
		lines, cached := overlay.cache[song]
		if !cached && !overlay.loading {
			overlay.loading = true
			folder := filepath.Join(filepath.Dir(a.statePath), "lyrics")
			version := overlay.cacheVersion
			go func() {
				lines, downloaded := overlay.diskCache.Load(context.Background(), folder, song)
				a.win.Update(func() {
					overlay.loading = false
					if version != overlay.cacheVersion {
						// An in-flight lookup can finish for playback without filling a cleared cache.
						if overlay.song != song {
							return
						}
					} else if downloaded {
						if err := overlay.diskCache.Save(song, lines); err != nil {
							log.Printf("cache lyrics: %v", err)
						} else if a.settingsWindow != nil {
							a.refreshLyricCacheSize()
						}
					}
					if overlay.cache == nil || len(overlay.cache) >= 32 {
						overlay.cache = make(map[lyrics.Song]lyrics.Lines)
					}
					overlay.cache[song] = lines
				})
			}()
		}
		if !a.expanded && frame.open == 0 && track.Rate > 0 {
			position := track.Elapsed(c.Now())
			var next float64
			rows, index, next = lines.Surrounding(position)
			if next > position && next <= track.Duration {
				delay := (next - position) / track.Rate
				c.After(max(time.Millisecond, time.Duration(delay*float64(time.Second))))
			}
			if rows[1] != "" {
				anchor = a.windowBounds(width, height)
				side = a.dock.side
			}
		}
	}
	if song != overlay.song || index != overlay.index || rows != overlay.rows {
		now := c.Now()
		// Start from settled rows when advancing to the next visible line of the same song.
		if song == overlay.song && index == overlay.index+1 && overlay.rows[1] != "" && rows[1] != "" &&
			now.Sub(overlay.shiftedAt) >= lyricTransition {
			overlay.outgoing, overlay.shiftedAt = overlay.rows[0], now
		} else {
			overlay.outgoing, overlay.shiftedAt = "", time.Time{}
		}
		overlay.song, overlay.index, overlay.rows = song, index, rows
	} else if anchor == overlay.anchor && side == overlay.side {
		return
	}
	overlay.anchor, overlay.side = anchor, side

	// Window creation and movement happen after the island's frame is built.
	a.win.Update(func() {
		var bounds mygo.Rectangle
		if overlay.rows[1] != "" && !a.settings.HideLyrics && a.win.IsVisible() && !a.expanded {
			display := a.dock.display
			if overlay.side == dockNone {
				display = mygo.Screen.DisplayMatching(overlay.anchor)
			}
			bounds = lyricBounds(overlay.anchor, display.WorkArea, overlay.side)
		}
		a.placeOverlay(&overlay.win, bounds, "歌词", overlay.view)
	})
}

func lyricBounds(island, area mygo.Rectangle, side dockSide) mygo.Rectangle {
	const gap = 10
	width, height := min(436, area.Width), min(94, area.Height)
	x, y := island.X+(island.Width-width)/2, island.Y+island.Height+gap
	switch side {
	case dockLeft:
		x = max(area.X, island.X+island.Width+gap)
		width = min(width, area.X+area.Width-x)
	case dockRight:
		right := min(area.X+area.Width, island.X-gap)
		width = min(width, right-area.X)
		x = right - width
	default:
		x = min(max(x, area.X), area.X+area.Width-width)
		if y+height > area.Y+area.Height {
			y = island.Y - gap - height
		}
	}
	if side != dockNone {
		y = island.Y + (island.Height-height)/2
	}
	y = min(max(y, area.Y), area.Y+area.Height-height)
	return mygo.Rectangle{X: x, Y: y, Width: width, Height: height}
}

func (o *lyricOverlay) view(c *ui.Context) {
	c.SetTheme(theme)
	c.Root().Background(ui.Transparent)
	align := ui.Center
	if o.side == dockLeft {
		align = ui.Start
	} else if o.side == dockRight {
		align = ui.End
	}
	progress := float32(1)
	if !o.shiftedAt.IsZero() && !c.Preferences().ReduceMotion {
		elapsed := c.Now().Sub(o.shiftedAt)
		if elapsed < lyricTransition {
			progress = ui.EaseOut(max(0, float32(elapsed)/float32(lyricTransition)))
			c.AnimationFrame()
		}
	}

	const rowSpacing = 28
	_, height := c.Size()
	lines := [4]string{o.outgoing, o.rows[0], o.rows[1], o.rows[2]}
	opacity := [4]float32{1 - progress, 1, 1, progress}
	highlight := [4]float32{0, 1 - progress, progress, 0}
	ui.Box(c.Key("lyrics")).Fill().Clip().Children(func() {
		for i, line := range lines {
			if line == "" || opacity[i] == 0 {
				continue
			}
			emphasis := highlight[i]
			rowHeight := 22 + 8*emphasis
			center := height/2 + (float32(i-2)+1-progress)*rowSpacing
			ui.Text(c.Key(i), line).Absolute().Left(10).Right(10).Top(center-rowHeight/2).Height(rowHeight).
				SingleLine().Ellipsis("…").TextAlign(align).Opacity(opacity[i]).
				FontSize(14+7*emphasis).FontWeight(500+int(200*emphasis)).
				TextColor(white.Alpha(0.58+0.42*emphasis)).
				TextShadow(0, 1, 4+2*emphasis, ink.Alpha(0.9+0.05*emphasis))
		}
	})
}
