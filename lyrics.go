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
	win     *mygo.Window
	cache   map[lyrics.Song]lyrics.Lines
	loading bool
	text    string
	anchor  mygo.Point
}

func (a *island) updateLyrics(c *ui.Context, frame islandFrame) {
	overlay := &a.lyrics
	track := a.media.track
	var text string
	var anchor mygo.Point
	if a.win.IsVisible() && track.Playing && track.Title != "" && track.Duration > 0 {
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
			text, next = lines.Current(position)
			if next > position && next <= track.Duration {
				delay := (next - position) / track.Rate
				c.After(max(time.Millisecond, time.Duration(delay*float64(time.Second))))
			}
			if text != "" {
				anchor = mygo.Point{X: a.centerX, Y: a.top + compactHeight}
			}
		}
	}
	if text == overlay.text && anchor == overlay.anchor {
		return
	}
	overlay.text, overlay.anchor = text, anchor

	// Window creation and movement happen after the island's frame is built.
	a.win.Update(func() {
		if overlay.text == "" || !a.win.IsVisible() || a.expanded {
			if overlay.win != nil {
				overlay.win.Hide()
			}
			return
		}
		const gap, height = 8, 52
		anchor := overlay.anchor
		area := mygo.Screen.DisplayNearestPoint(anchor).WorkArea
		width := min(436, area.Width)
		x := min(max(anchor.X-width/2, area.X), area.X+area.Width-width)
		y := anchor.Y + gap
		if y+height > area.Y+area.Height {
			y = max(area.Y, anchor.Y-compactHeight-gap-height)
		}
		bounds := mygo.Rectangle{X: x, Y: y, Width: width, Height: height}
		if overlay.win == nil {
			overlay.win = mygo.NewWindow(mygo.WindowOptions{
				Title: "歌词", Parent: a.win, Content: ui.View(func(c *ui.Context) {
					c.Root().Background(ui.Transparent)
					ui.Box(c.Key("lyrics")).Fill().Padding(2, 12).Center().Children(func() {
						ui.Text(c, overlay.text).FillWidth().FontSize(17).FontWeight(600).
							TextAlign(ui.Center).Wrap().MaxLines(2).TextColor(white).
							TextShadow(0, 1, 5, ink.Alpha(0.94))
					})
				}),
				X: x, Y: y, Width: width, Height: height,
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
