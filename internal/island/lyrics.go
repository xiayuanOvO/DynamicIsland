package island

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

const (
	lyricTransition    = 240 * time.Millisecond
	lyricOverlayHeight = 120
)

type lyricOverlay struct {
	win       *mygo.Window
	cache     map[lyrics.Song]lyrics.Lines
	loading   bool
	song      lyrics.Song
	index     int
	rows      [3]string
	outgoing  string
	shiftedAt time.Time
	anchor    mygo.Rectangle
	side      dockSide
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
	width, height := min(436, area.Width), min(lyricOverlayHeight, area.Height)
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

	const (
		inset           = 10
		rowSpacing      = 28
		adjacentHeight  = 22
		currentHeight   = 30
		extraLineHeight = 26
		currentSize     = 21
		currentWeight   = 700
	)
	width, height := c.Size()
	lines := [4]string{o.outgoing, o.rows[0], o.rows[1], o.rows[2]}
	opacity := [4]float32{1 - progress, 1, 1, progress}
	highlight := [4]float32{0, 1 - progress, progress, 0}
	activeHeight := [4]float32{adjacentHeight, currentHeight, currentHeight, adjacentHeight}
	for i := 1; i <= 2; i++ {
		if lines[i] == "" || highlight[i] == 0 {
			continue
		}
		textWidth, _ := c.MeasureText(0, ui.Span{Text: lines[i], Size: currentSize, Weight: currentWeight})
		if textWidth > max(1, width-2*inset) {
			activeHeight[i] += extraLineHeight
		}
	}
	beforeSpacing := rowSpacing + (activeHeight[1]-currentHeight)/2
	afterSpacing := rowSpacing + (activeHeight[2]-currentHeight)/2
	// Move between the two layouts while reserving space for a wrapped current line.
	before := [4]float32{-beforeSpacing, 0, beforeSpacing, beforeSpacing + rowSpacing}
	after := [4]float32{-afterSpacing - rowSpacing, -afterSpacing, 0, afterSpacing}
	ui.Box(c.Key("lyrics")).Fill().Clip().Children(func() {
		for i, line := range lines {
			if line == "" || opacity[i] == 0 {
				continue
			}
			emphasis := highlight[i]
			rowHeight := adjacentHeight + (activeHeight[i]-adjacentHeight)*emphasis
			center := height/2 + before[i]*(1-progress) + after[i]*progress
			text := ui.Text(c.Key(i), line).Absolute().Left(inset).Right(inset).Top(center-rowHeight/2).
				TextAlign(align).Opacity(opacity[i]).
				FontSize(14+(currentSize-14)*emphasis).FontWeight(500+int((currentWeight-500)*emphasis)).
				TextColor(white.Alpha(0.58+0.42*emphasis)).
				TextShadow(0, 1, 4+2*emphasis, ink.Alpha(0.9+0.05*emphasis))
			// While scrolling, open the second line once there is room for it to render whole.
			if activeHeight[i] > currentHeight && emphasis >= 0.6 {
				text.MaxLines(2).FixedLineHeight(min(extraLineHeight, rowHeight/2))
			} else {
				text.SingleLine()
			}
		}
	})
}
