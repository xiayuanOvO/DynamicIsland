package island

import (
	"context"
	"log"
	"math"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"dynamicisland/internal/lyrics"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	lyricTransition    = 240 * time.Millisecond
	lyricOverlayHeight = 120
	lyricScrollDelay   = 500 * time.Millisecond
	lyricScrollHold    = 250 * time.Millisecond
	lyricScrollMaxRate = 280 // DIPs per second
)

type lyricOverlay struct {
	win            *mygo.Window
	cache          map[lyrics.Song]lyrics.Lines
	diskCache      lyrics.Cache
	cacheVersion   uint64
	cacheBytes     int64
	cacheSizeErr   error
	cacheClearErr  error
	loading        bool
	song           lyrics.Song
	index          int
	rows           [3]string
	outgoing       string
	currentSince   time.Time
	lineEndsAt     time.Time
	scroll         lyricScroll
	previousOffset float32
	shiftedAt      time.Time
	anchor         mygo.Rectangle
	side           dockSide
}

func (a *island) updateLyrics(c *ui.Context, frame islandFrame, width, height int) {
	overlay := &a.lyrics
	track := a.media.track
	now := c.Now()
	var song lyrics.Song
	index := -1
	var rows [3]string
	var anchor mygo.Rectangle
	var side dockSide
	var lineEndsAt time.Time
	if !a.settings.HideLyrics && a.win.IsVisible() && track.Playing && track.Title != "" && track.HasPosition() {
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
			position := track.Elapsed(now)
			var next float64
			rows, index, next = lines.Surrounding(position)
			if next > position && (track.Duration <= 0 || next <= track.Duration) {
				delay := (next - position) / track.Rate
				c.After(max(time.Millisecond, time.Duration(delay*float64(time.Second))))
			}
			if rows[1] != "" {
				end := track.Duration
				if next > position && (end <= 0 || next < end) {
					end = next
				}
				if end > 0 {
					// Anchor to the player sample so repainting alone does not change the deadline.
					lineEndsAt = track.Sampled.Add(time.Duration((end - track.Position) / track.Rate * float64(time.Second)))
				}
				anchor = a.windowBounds(width, height)
				side = a.dock.side
			}
		}
	}
	if song != overlay.song || index != overlay.index || rows != overlay.rows {
		// Start from settled rows when advancing to the next visible line of the same song.
		if song == overlay.song && index == overlay.index+1 && overlay.rows[1] != "" && rows[1] != "" &&
			now.Sub(overlay.shiftedAt) >= lyricTransition {
			overlay.outgoing, overlay.shiftedAt = overlay.rows[0], now
			overlay.previousOffset = overlay.scroll.offset(now)
		} else {
			overlay.outgoing, overlay.shiftedAt = "", time.Time{}
		}
		overlay.currentSince = now
		overlay.scroll = lyricScroll{}
		overlay.song, overlay.index, overlay.rows = song, index, rows
	} else if anchor == overlay.anchor && side == overlay.side && lineEndsAt.Equal(overlay.lineEndsAt) {
		return
	}
	overlay.anchor, overlay.side, overlay.lineEndsAt = anchor, side, lineEndsAt

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
	now := c.Now()
	reduceMotion := c.Preferences().ReduceMotion
	if !o.shiftedAt.IsZero() && !reduceMotion {
		elapsed := now.Sub(o.shiftedAt)
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
	textWidth := max(1, width-2*inset)
	lines := [4]string{o.outgoing, o.rows[0], o.rows[1], o.rows[2]}
	opacity := [4]float32{1 - progress, 1, 1, progress}
	highlight := [4]float32{0, 1 - progress, progress, 0}
	activeHeight := [4]float32{adjacentHeight, currentHeight, currentHeight, adjacentHeight}
	for i := 1; i <= 2; i++ {
		if lines[i] == "" || highlight[i] == 0 {
			continue
		}
		lineWidth, _ := c.MeasureText(0, ui.Span{Text: lines[i], Size: currentSize, Weight: currentWeight})
		if lineWidth > textWidth {
			activeHeight[i] += extraLineHeight
		}
	}
	beforeSpacing := rowSpacing + (activeHeight[1]-currentHeight)/2
	afterSpacing := rowSpacing + (activeHeight[2]-currentHeight)/2
	// Move between the two layouts while reserving space for a wrapped current line.
	before := [4]float32{-beforeSpacing, 0, beforeSpacing, beforeSpacing + rowSpacing}
	after := [4]float32{-afterSpacing - rowSpacing, -afterSpacing, 0, afterSpacing}
	root := ui.Box(c.Key("lyrics")).Fill().Clip()
	marquee := ui.Local(root, "marquee", func() lyricMarquee { return lyricMarquee{} })
	root.Children(func() {
		for i, line := range lines {
			if line == "" || opacity[i] == 0 {
				continue
			}
			emphasis := highlight[i]
			rowHeight := adjacentHeight + (activeHeight[i]-adjacentHeight)*emphasis
			center := height/2 + before[i]*(1-progress) + after[i]*progress
			wrapped := activeHeight[i] > currentHeight && emphasis >= 0.6
			size, weight := 14+(currentSize-14)*emphasis, 500+int((currentWeight-500)*emphasis)
			top, lineHeight := center-rowHeight/2, min(extraLineHeight, rowHeight/2)
			style := func(e ui.Element) ui.Element {
				return e.Absolute().Left(inset).Right(inset).Top(top).
					TextAlign(align).Opacity(opacity[i]).FontSize(size).FontWeight(weight).
					TextColor(white.Alpha(0.58+0.42*emphasis)).
					TextShadow(0, 1, 4+2*emphasis, ink.Alpha(0.9+0.05*emphasis))
			}
			if wrapped && !reduceMotion {
				font := ui.Font{Family: theme.Font, Size: currentSize, Weight: currentWeight, Lang: c.Locale()}
				marquee.split(line, textWidth, font, c.Direction())
				if marquee.tail != "" {
					tailWidth, _ := c.MeasureText(0, ui.Span{Text: marquee.tail, Size: size, Weight: weight})
					overflow := max(0, tailWidth-textWidth)
					var distance float32
					if i == 1 {
						distance = min(overflow, o.previousOffset)
					} else if !o.lineEndsAt.IsZero() {
						o.scroll.retarget(now, o.currentSince, o.lineEndsAt, overflow)
						distance = o.scroll.offset(now)
						if distance < overflow {
							if delay := o.scroll.start.Sub(now); delay > 0 {
								c.After(delay)
							} else {
								c.AnimationFrame()
							}
						}
					}
					x := -distance
					if marquee.rtl {
						x = distance - overflow
					}
					style(ui.Box(c.Key(i))).Height(2 * lineHeight).ClipX().Role(ui.RoleText).Label(line).Children(func() {
						ui.Text(c, marquee.first).FillWidth().NoWrap().FixedLineHeight(lineHeight).AccessHidden(true)
						ui.Text(c, marquee.tail).Absolute().Left(x).Top(lineHeight).Width(max(textWidth, tailWidth)).
							NoWrap().FixedLineHeight(lineHeight).AccessHidden(true)
					})
					continue
				}
			}
			text := style(ui.Text(c.Key(i), line))
			if wrapped {
				text.MaxLines(2).FixedLineHeight(lineHeight)
			} else {
				text.SingleLine()
			}
		}
	})
}

type lyricScroll struct {
	start, finish, deadline time.Time
	from, overflow          float32
}

func (s lyricScroll) offset(now time.Time) float32 {
	if s.finish.IsZero() || !now.After(s.start) {
		return s.from
	}
	if !now.Before(s.finish) {
		return s.overflow
	}
	progress := float32(now.Sub(s.start)) / float32(s.finish.Sub(s.start))
	return s.from + (s.overflow-s.from)*progress
}

func (s *lyricScroll) retarget(now, shownAt, deadline time.Time, overflow float32) {
	if deadline.Equal(s.deadline) && overflow == s.overflow {
		return
	}
	from := min(overflow, s.offset(now))
	fastest := time.Duration(float64(overflow-from) / lyricScrollMaxRate * float64(time.Second))
	// Shorten both reading pauses when there is little time at the maximum speed.
	spare := max(0, deadline.Sub(now)-fastest)
	var delay time.Duration
	if from == 0 {
		delay = min(spare/2, max(0, shownAt.Add(lyricScrollDelay).Sub(now)))
	}
	hold := min(lyricScrollHold, spare-delay)
	start := now.Add(delay)
	finish := start.Add(fastest + spare - delay - hold)
	*s = lyricScroll{start: start, finish: finish, deadline: deadline, from: from, overflow: overflow}
}

type lyricMarquee struct {
	line, first, tail string
	width             float32
	font              ui.Font
	direction         ui.Direction
	rtl               bool
}

func (m *lyricMarquee) split(line string, width float32, font ui.Font, direction ui.Direction) {
	if m.line == line && m.width == width && m.font == font && m.direction == direction {
		return
	}
	*m = lyricMarquee{line: line, width: width, font: font, direction: direction}
	layout := ui.ShapeRichTextWith([]ui.Span{{Text: line}}, font, ui.TextShape{Width: width, Direction: direction})
	lines := layout.Lines()
	if len(lines) <= 2 {
		return
	}
	// MyGo's line offsets are UTF-16. Split at the start of the second shaped line.
	offset := lines[1].Start
	for i, r := range line {
		if offset == 0 {
			m.first, m.tail = strings.TrimSpace(line[:i]), strings.TrimSpace(line[i:])
			m.rtl = lines[1].RTL
			return
		}
		offset -= utf16.RuneLen(r)
	}
}
