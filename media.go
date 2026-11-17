package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"dynamicisland/internal/audiolevel"
	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

const mediaCardHeight = 110

type islandMedia struct {
	adapter          string
	track            nowplaying.Track
	artwork          ui.ImageSource
	artworkHash      [32]byte
	artworkLoading   bool
	refreshAt        time.Time
	fastRefreshUntil time.Time
	reading          bool
	action           nowplaying.Action
	pending          nowplaying.Command
	scrubbing        bool
	scrub            float64
	capture          *audiolevel.Capture
	wave             audiolevel.Bars
	waveFrameAt      time.Time
}

var (
	playIcon     = svg(`<path d="m8 5 12 7-12 7V5Z" fill="currentColor"/>`)
	pauseIcon    = svg(`<path d="M9 6v12m6-12v12" stroke-width="3"/>`)
	previousIcon = svg(`<path d="M5 6v12m13-12L8 12l10 6V6Z"/>`)
	nextIcon     = svg(`<path d="M19 6v12M6 6l10 6-10 6V6Z"/>`)
)

func (a *island) updateMedia(c *ui.Context) {
	m := &a.media
	if m.adapter == "" || !a.win.IsVisible() || m.reading || m.action != "" {
		return
	}
	if delay := m.refreshAt.Sub(c.Now()); delay > 0 {
		c.After(delay)
		return
	}
	a.requestMedia(nowplaying.Command{})
}

func (a *island) requestMedia(command nowplaying.Command) {
	m := &a.media
	if m.reading {
		m.pending = command
		return
	}
	if m.action != "" {
		return
	}
	m.reading = command.Action == ""
	m.action = command.Action
	adapter, oldHash, oldSong, withArtwork := m.adapter, m.artworkHash, m.track, a.expanded
	go func() {
		var controlErr error
		if command.Action != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			controlErr = nowplaying.Control(ctx, adapter, command)
			cancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		track, readErr := nowplaying.Read(ctx, adapter, withArtwork)
		cancel()
		hash := sha256.Sum256(track.Artwork)
		var cover ui.ImageSource
		if len(track.Artwork) > 0 && len(track.Artwork) <= 8<<20 && (hash != oldHash || !oldSong.SameSong(track)) {
			cover, _ = ui.DecodeImage(track.Artwork)
		}
		track.Artwork = nil
		a.win.Update(func() {
			m.reading, m.action = false, ""
			now := time.Now()
			if controlErr == nil && (command.Action == nowplaying.Next || command.Action == nowplaying.Previous) {
				m.fastRefreshUntil = now.Add(1500 * time.Millisecond)
			}
			m.refreshAt = now.Add(2 * time.Second)
			fastRefresh := now.Before(m.fastRefreshUntil)
			if fastRefresh {
				m.refreshAt = now.Add(150 * time.Millisecond)
			}
			trackChanged := false
			if readErr == nil && (!fastRefresh || (track.Title != "" && (track.Playing || track.Source == m.track.Source))) {
				trackChanged = !m.track.SameSong(track)
				if trackChanged {
					m.scrubbing = false
					m.artwork, m.artworkHash = nil, [32]byte{}
				}
				m.track = track
				if cover != nil {
					m.artwork, m.artworkHash = cover, hash
				}
				if load := track.ArtworkLoader; load != nil && a.expanded && m.artwork == nil && !m.artworkLoading {
					m.artworkLoading = true
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancel()
						cover, _ := ui.DecodeImage(load(ctx))
						a.win.Update(func() {
							m.artworkLoading = false
							if m.artwork == nil && m.track.SameSong(track) {
								m.artwork = cover
							}
						})
					}()
				}
			}
			if controlErr != nil {
				a.message("无法控制播放", true)
			}
			if pending := m.pending; pending.Action != "" {
				m.pending = nowplaying.Command{}
				if pending.Source == m.track.Source && (pending.Action != nowplaying.Seek || !trackChanged) {
					a.requestMedia(pending)
				}
			}
		})
	}()
}

func (a *island) updateWave(c *ui.Context) {
	m := &a.media
	if !a.win.IsVisible() || m.track.Title == "" {
		m.stopWave()
		return
	}
	var levels audiolevel.Bars
	var running bool
	if m.track.Playing {
		if m.capture == nil {
			m.capture = audiolevel.Start()
		}
		levels, running = m.capture.Levels()
	} else if m.capture != nil {
		m.capture.Stop()
		m.capture = nil
	}
	if !running && m.wave == (audiolevel.Bars{}) {
		m.waveFrameAt = time.Time{}
		return
	}
	now := c.Now()
	elapsed := now.Sub(m.waveFrameAt)
	if m.waveFrameAt.IsZero() {
		elapsed = time.Second / 60
	}
	m.wave.Update(levels, elapsed)
	m.waveFrameAt = now
	c.AnimationFrame()
}

func (m *islandMedia) stopWave() {
	if m.capture != nil {
		m.capture.Stop()
		m.capture = nil
	}
	m.wave = audiolevel.Bars{}
	m.waveFrameAt = time.Time{}
}

func (a *island) mediaCard(c *ui.Context) {
	m := &a.media
	track := m.track
	control := func(action nowplaying.Action, position float64) {
		a.requestMedia(nowplaying.Command{Action: action, Source: track.Source, Position: position})
	}
	position := track.Elapsed(c.Now())
	if m.action == nowplaying.Seek {
		position = m.scrub
	} else if m.pending.Action == nowplaying.Seek {
		position = m.pending.Position
	}
	if track.Playing && a.expanded {
		c.After(200 * time.Millisecond)
	}
	ui.Column(c.Key("media")).FillWidth().Height(mediaCardHeight).Shrink(0).
		Padding(10, 12).Gap(5).Radius(16).
		Gradient(white.Alpha(0.48), white.Alpha(0.19), 155).Border(0.75, white.Alpha(0.65)).
		InsetShadow(0, 1, 0, 0, white.Alpha(0.46)).Children(func() {
		ui.Row(c.Key("track")).FillWidth().Height(40).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c.Key("cover")).Size(40, 40).Shrink(0).Radius(9).Clip().
				Background(white.Alpha(0.4)).Center().Children(func() {
				if m.artwork != nil {
					ui.Image(c, m.artwork).Fill().Fit(ui.Cover)
				} else {
					ui.Icon(c, mediaIcon).Size(20, 20).TextColor(muted)
				}
			})
			ui.Column(c.Key("metadata")).Grow(1).MinWidth(0).Gap(3).Clip().Children(func() {
				ui.Text(c, track.Title).FillWidth().SingleLine().FontSize(12).FontWeight(600).
					TextColor(ink).Tooltip(track.Title)
				if track.Artist != "" {
					ui.Text(c, track.Artist).FillWidth().SingleLine().FontSize(10.5).
						TextColor(muted).Tooltip(track.Artist)
				}
			})
			if iconButton(c.Key("previous"), previousIcon, "上一首").Disabled(!track.Previous || m.action != "").Clicked() {
				control(nowplaying.Previous, 0)
			}
			icon, label := playIcon, "播放"
			if track.Playing {
				icon, label = pauseIcon, "暂停"
			}
			if iconButton(c.Key("toggle"), icon, label).Size(34, 34).Radius(17).
				Background(theme.SurfaceHover).TextColor(ink).
				Disabled(!track.Toggle || m.action != "").Clicked() {
				control(nowplaying.Toggle, 0)
			}
			if iconButton(c.Key("next"), nextIcon, "下一首").Disabled(!track.Next || m.action != "").Clicked() {
				control(nowplaying.Next, 0)
			}
		})

		var wave ui.Element
		if track.CanSeek() {
			if !m.scrubbing {
				m.scrub = position
			}
			wave = ui.SliderBase(c.Key("seek"), &m.scrub, 0, track.Duration).
				Cursor(ui.CursorPointer).Label("播放进度").Disabled(m.action != "" || !a.expanded)
			changed := wave.Changed()
			button, pressed := wave.PressedButton()
			held := pressed && button == 0
			if m.scrubbing || changed {
				position = m.scrub
				if !held {
					control(nowplaying.Seek, position)
				}
				m.scrubbing = held
			}
		} else {
			wave = ui.Box(c.Key("waveform"))
			m.scrubbing = false
		}
		fraction := float32(0)
		if track.Duration > 0 {
			fraction = float32(position / track.Duration)
		}
		wave.FillWidth().Height(24).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			paintMediaBars(p, r, m.wave[:], 1, fraction)
		})
		ui.Row(c.Key("times")).FillWidth().Height(14).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, mediaTime(position)).FontSize(10).TextColor(muted)
			ui.Box(c).Grow(1)
			if track.Duration > 0 {
				ui.Text(c, mediaTime(track.Duration)).FontSize(10).TextColor(muted)
			}
		})
	})
}

func paintMediaBars(p *ui.Painter, r ui.Rect, levels []float32, bandsPerBar int, fraction float32) {
	bars := float32(len(levels) / bandsPerBar)
	step := r.W / bars
	for i := 0; i < len(levels); i += bandsPerBar {
		x := float32(i / bandsPerBar)
		level := levels[i]
		for _, band := range levels[i+1 : i+bandsPerBar] {
			level = max(level, band)
		}
		height := float32(5) + 17*level
		fill := ink.Alpha(0.17)
		if (x+0.5)/bars <= fraction {
			fill = accent.Alpha(0.88)
		}
		p.Fill(ui.Rect{X: r.X + x*step + 1, Y: r.Y + (r.H-height)/2, W: max(0, step-2), H: height}, fill, 2)
	}
}

func mediaTime(seconds float64) string {
	s := int(seconds)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
