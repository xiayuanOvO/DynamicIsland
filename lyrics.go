package main

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"time"

	"dynamicisland/internal/lyrics"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type lyricOverlay struct {
	win          *mygo.Window
	cache        map[lyrics.Song]lyrics.Lines
	loading      bool
	rows         [3]string
	currentSince time.Time
	anchor       mygo.Rectangle
	side         dockSide
}

func (a *island) updateLyrics(c *ui.Context, frame islandFrame, width, height int) {
	overlay := &a.lyrics
	track := a.media.track
	var rows [3]string
	var anchor mygo.Rectangle
	var side dockSide
	if !a.settings.HideLyrics && a.win.IsVisible() && track.Playing && track.Title != "" && track.Duration > 0 {
		song := lyrics.Song{
			Title: strings.TrimSpace(track.Title), Artist: strings.TrimSpace(track.Artist),
			Album: strings.TrimSpace(track.Album), Duration: math.Round(track.Duration),
		}
		lines, cached := overlay.cache[song]
		if !cached && !overlay.loading {
			overlay.loading = true
			folder := filepath.Join(filepath.Dir(a.statePath), "lyrics")
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				lines := lyrics.Load(ctx, folder, song)
				a.win.Update(func() {
					if overlay.cache == nil || len(overlay.cache) >= 32 {
						overlay.cache = make(map[lyrics.Song]lyrics.Lines)
					}
					overlay.cache[song] = lines
					overlay.loading = false
				})
			}()
		}
		if !a.expanded && frame.open == 0 && track.Rate > 0 {
			position := track.Elapsed(c.Now())
			var next float64
			rows, next = lines.Surrounding(position)
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
	if rows == overlay.rows && anchor == overlay.anchor && side == overlay.side {
		return
	}
	if rows[1] != overlay.rows[1] {
		overlay.currentSince = c.Now()
	}
	overlay.rows, overlay.anchor, overlay.side = rows, anchor, side

	// Window creation and movement happen after the island's frame is built.
	a.win.Update(func() {
		if overlay.rows[1] == "" || a.settings.HideLyrics || !a.win.IsVisible() || a.expanded {
			if overlay.win != nil {
				overlay.win.Hide()
			}
			return
		}
		display := a.dock.display
		if overlay.side == dockNone {
			display = mygo.Screen.DisplayMatching(overlay.anchor)
		}
		bounds := lyricBounds(overlay.anchor, display.WorkArea, overlay.side)
		if bounds.Width <= 0 || bounds.Height <= 0 {
			if overlay.win != nil {
				overlay.win.Hide()
			}
			return
		}
		if overlay.win == nil {
			overlay.win = mygo.NewWindow(mygo.WindowOptions{
				Title: "歌词", Parent: a.win, Content: ui.View(overlay.view),
				X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
				Hidden: true, Frameless: true, Transparent: true, Panel: true,
				AlwaysOnTop: true, SkipTaskbar: true, IgnoreMouseEvents: true,
				DisableResize: true, DisableShadow: true,
				BackgroundColor: "#00000000",
			})
		} else {
			if overlay.win.Bounds() != bounds {
				overlay.win.SetBounds(bounds)
			}
			overlay.win.Invalidate()
		}
		if !overlay.win.IsVisible() {
			overlay.win.ShowInactive()
		}
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
	const transition = 200 * time.Millisecond
	mix := ui.EaseOut(min(1, max(0, float32(c.Now().Sub(o.currentSince))/float32(transition))))
	if mix < 1 {
		c.AnimationFrame()
	}
	ui.Column(c.Key("lyrics")).Fill().Padding(3, 10).Gap(2).Justify(ui.Center).Children(func() {
		for index, line := range o.rows {
			row := ui.Text(c.Key(index), line).FillWidth().SingleLine().Ellipsis("…").
				TextAlign(align).Shrink(0)
			if index == 1 {
				row.Height(30).FontSize(21).FontWeight(700).TextColor(white).
					TextShadow(0, 1, 6, ink.Alpha(0.95)).Opacity(0.72+0.28*mix).Translate(0, 4*(1-mix))
			} else {
				row.Height(22).FontSize(14).FontWeight(500).TextColor(white.Alpha(0.58)).
					TextShadow(0, 1, 4, ink.Alpha(0.9))
			}
		}
	})
}
