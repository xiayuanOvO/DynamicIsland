package island

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"log"
	"time"

	"dynamicisland/internal/audiolevel"
	"dynamicisland/internal/lyrics"
	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

const (
	mediaCardHeight     = 110
	largeMediaCoverSize = 132
	maxArtworkDimension = largeMediaCoverSize * 2
)

func (s appSettings) cardHeight() int {
	if s.MediaCardStyle == mediaCardLargeCover {
		return largeMediaCoverSize + 24
	}
	return mediaCardHeight
}

type islandMedia struct {
	adapter          string
	track            nowplaying.Track
	catalogDuration  float64
	artwork          ui.ImageSource
	artworkColor     ui.Color
	artworkHash      [32]byte
	artworkCacheDir  string
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
	adapter, oldHash, oldSong, cacheDir := m.adapter, m.artworkHash, m.track, m.artworkCacheDir
	withArtwork := m.artwork == nil || oldSong.Title == "" || command.Action != ""
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
		cover, coverColor, hash, keepCover := resolveArtwork(track, oldSong, oldHash, cacheDir)
		track.Artwork = nil
		a.win.Update(func() {
			m.reading, m.action = false, ""
			now := time.Now()
			skipping := command.Action == nowplaying.Next || command.Action == nowplaying.Previous
			startingWithoutMetadata := command.Action == nowplaying.Toggle && oldSong.Title != "" && !oldSong.Playing && readErr == nil && track.Title == ""
			if controlErr == nil && (skipping || startingWithoutMetadata) {
				m.fastRefreshUntil = now.Add(1500 * time.Millisecond)
			}
			m.refreshAt = now.Add(2 * time.Second)
			fastRefresh := now.Before(m.fastRefreshUntil)
			if fastRefresh {
				m.refreshAt = now.Add(150 * time.Millisecond)
			}
			trackChanged := false
			if readErr == nil && (!fastRefresh || (track.Title != "" && (track.Playing || track.Source == m.track.Source))) {
				track.EstimateNetEasePosition(m.track)
				trackChanged = !m.track.SameSong(track)
				if trackChanged {
					m.scrubbing = false
					m.catalogDuration = 0
					if track.Duration <= 0 && track.IsNetEase() && track.Artist != "" {
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
							defer cancel()
							duration := lyrics.NetEaseDuration(ctx, lyrics.Song{Title: track.Title, Artist: track.Artist, Album: track.Album})
							if duration > 0 {
								a.win.Update(func() {
									if m.track.SameSong(track) {
										m.catalogDuration = duration
										if a.settingsWindow != nil {
											a.settingsWindow.Invalidate()
										}
									}
								})
							}
						}()
					}
				}
				m.track = track
				if a.settingsWindow != nil && oldSong.Playing != track.Playing {
					a.settingsWindow.Invalidate()
				}
				if cover != nil {
					a.setArtwork(cover, coverColor, hash)
				} else if !keepCover && m.artwork != nil {
					a.setArtwork(nil, ui.Transparent, [32]byte{})
				}
				if (track.Playing || a.expanded) && !withArtwork && m.artwork == nil {
					m.refreshAt = now
				}
				if load := track.ArtworkLoader; load != nil && (a.expanded || track.Playing) && m.artwork == nil && !m.artworkLoading {
					m.artworkLoading = true
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancel()
						data := load(ctx)
						cover, color := decodeArtwork(data)
						if cover != nil {
							saveArtwork(cacheDir, track.ArtworkKey(), data)
						}
						hash := sha256.Sum256(data)
						a.win.Update(func() {
							m.artworkLoading = false
							if cover != nil && m.artwork == nil && m.track.ArtworkKey() == track.ArtworkKey() {
								a.setArtwork(cover, color, hash)
							}
						})
					}()
				}
			}
			if controlErr != nil {
				log.Printf("media control: %v", controlErr)
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

func (a *island) setArtwork(cover ui.ImageSource, color ui.Color, hash [32]byte) {
	if a.settingsWindow != nil {
		a.settingsWindow.Invalidate()
	}
	a.media.artwork, a.media.artworkColor, a.media.artworkHash = cover, color, hash
}

func decodeArtwork(data []byte) (ui.ImageSource, ui.Color) {
	if len(data) == 0 || len(data) > maxArtworkBytes {
		return nil, ui.Transparent
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		cover, _ := ui.DecodeImage(data)
		return cover, ui.Transparent
	}
	color := dominantArtworkColor(img)
	scaled := downscaleArtwork(img, maxArtworkDimension)
	return ui.NewBitmap(scaled), color
}

func downscaleArtwork(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || (sw <= maxDim && sh <= maxDim) {
		return src
	}
	dw, dh := maxDim, maxDim
	if sw >= sh {
		dh = max(1, sh*maxDim/sw)
	} else {
		dw = max(1, sw*maxDim/sh)
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		sy := b.Min.Y + y*sh/dh
		for x := range dw {
			sx := b.Min.X + x*sw/dw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func dominantArtworkColor(img image.Image) ui.Color {
	type sample struct{ weight, red, green, blue uint32 }
	var bins [512]sample
	var dominant sample
	bounds := img.Bounds()
	columns, rows := min(bounds.Dx(), 24), min(bounds.Dy(), 24)
	for row := range rows {
		y := bounds.Min.Y + (2*row+1)*bounds.Dy()/(2*rows)
		for col := range columns {
			x := bounds.Min.X + (2*col+1)*bounds.Dx()/(2*columns)
			red, green, blue, alpha := img.At(x, y).RGBA()
			if alpha < 0x8000 {
				continue
			}
			red, green, blue = red*255/alpha, green*255/alpha, blue*255/alpha
			weight := 64 + max(red, green, blue) - min(red, green, blue)
			bin := &bins[(red>>5)*64+(green>>5)*8+(blue>>5)]
			bin.weight += weight
			bin.red += red * weight
			bin.green += green * weight
			bin.blue += blue * weight
			if bin.weight > dominant.weight {
				dominant = *bin
			}
		}
	}
	if dominant.weight == 0 {
		return ui.Transparent
	}
	return ui.RGB(uint8(dominant.red/dominant.weight), uint8(dominant.green/dominant.weight), uint8(dominant.blue/dominant.weight))
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

func (m *islandMedia) cover(c *ui.Context, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Radius(min(18, size*0.225)).Clip().Label("专辑封面").
		Background(colorsFor(c.Theme().Dark).glass.Alpha(0.4)).Center().Children(func() {
		if m.artwork != nil {
			ui.Image(c, m.artwork).Fill().Fit(ui.Cover)
		} else {
			ui.Icon(c, mediaIcon).Size(size/2, size/2).TextColor(c.Theme().TextMuted)
		}
	})
}

func (a *island) mediaCard(c *ui.Context) {
	source := a.media.track.Source
	a.settings.renderMediaCard(c, &a.media, a.expanded, func(action nowplaying.Action, position float64) {
		a.requestMedia(nowplaying.Command{Action: action, Source: source, Position: position})
	})
}

func (a *island) mediaCardPreview(c *ui.Context) {
	preview := islandMedia{track: a.media.track, artwork: a.media.artwork, artworkColor: a.media.artworkColor, catalogDuration: a.media.catalogDuration}
	if preview.track.Title == "" {
		preview.track = nowplaying.Track{
			Title: "示例曲目", Artist: "示例艺人", Position: 63, Duration: 228,
			Playing: true, Previous: true, Next: true, Toggle: true,
		}
	}
	for i := range preview.wave {
		preview.wave[i] = [...]float32{0.18, 0.38, 0.52, 0.30, 0.60, 0.46, 0.22}[i%7]
	}
	a.settings.renderMediaCard(c, &preview, false, nil)
}

func (s appSettings) renderMediaCard(c *ui.Context, m *islandMedia, expanded bool, control func(nowplaying.Action, float64)) {
	track := m.track
	large := s.MediaCardStyle == mediaCardLargeCover
	var position float64
	if !track.IsNetEase() || track.HasPosition() {
		position = track.Elapsed(c.Now())
	}
	duration := track.Duration
	if duration <= 0 && m.catalogDuration > 0 {
		duration = m.catalogDuration
		position = min(position, duration)
	}
	if m.action == nowplaying.Seek {
		position = m.scrub
	} else if m.pending.Action == nowplaying.Seek {
		position = m.pending.Position
	}
	if control != nil && track.Playing && expanded {
		c.After(200 * time.Millisecond)
	}
	metadata := func() {
		details := ui.Column(c.Key("metadata")).MinWidth(0).Gap(3).Clip()
		titleSize, artistSize := float32(12), float32(10.5)
		if large {
			details.FillWidth()
			titleSize, artistSize = 16, 12
		} else {
			details.Grow(1)
		}
		details.Children(func() {
			ui.Text(c, track.Title).FillWidth().SingleLine().FontSize(titleSize).FontWeight(600).
				TextColor(c.Theme().Text).Tooltip(track.Title)
			if track.Artist != "" {
				ui.Text(c, track.Artist).FillWidth().SingleLine().FontSize(artistSize).
					TextColor(c.Theme().TextMuted).Tooltip(track.Artist)
			}
		})
	}
	controls := func() {
		if iconButton(c.Key("previous"), previousIcon, "上一首").Disabled(!track.Previous || m.action != "").Clicked() && control != nil {
			control(nowplaying.Previous, 0)
		}
		icon, label := playIcon, "播放"
		if track.Playing {
			icon, label = pauseIcon, "暂停"
		}
		if iconButton(c.Key("toggle"), icon, label).Size(34, 34).Radius(17).
			Background(c.Theme().SurfaceHover).TextColor(c.Theme().Text).
			Disabled(!track.Toggle || m.action != "").Clicked() && control != nil {
			control(nowplaying.Toggle, 0)
		}
		if iconButton(c.Key("next"), nextIcon, "下一首").Disabled(!track.Next || m.action != "").Clicked() && control != nil {
			control(nowplaying.Next, 0)
		}
	}
	progress := func() {
		var wave ui.Element
		if control != nil && track.CanSeek() {
			if !m.scrubbing {
				m.scrub = position
			}
			wave = ui.SliderBase(c.Key("seek"), &m.scrub, 0, track.Duration).
				Cursor(ui.CursorPointer).Label("播放进度").Disabled(m.action != "" || !expanded)
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
		if duration > 0 {
			fraction = float32(position / duration)
		}
		waveHeight := float32(24)
		if large {
			waveHeight = 14
		}
		wave.FillWidth().Height(waveHeight).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			color := s.waveColor(m.artworkColor, c.Theme().Dark)
			if large {
				bar := ui.Rect{X: r.X, Y: r.Y + (r.H-4)/2, W: r.W, H: 4}
				p.Fill(bar, p.Theme().Text.Alpha(0.13), 2)
				bar.W *= min(1, max(0, fraction))
				p.Fill(bar, color.Alpha(0.88), min(2, bar.W/2))
			} else {
				paintMediaBars(p, r, m.wave[:], 1, fraction, color)
			}
		})
		ui.Row(c.Key("times")).FillWidth().Height(14).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, mediaTime(position)).FontSize(10).TextColor(c.Theme().TextMuted)
			ui.Box(c).Grow(1)
			if duration > 0 {
				ui.Text(c, mediaTime(duration)).FontSize(10).TextColor(c.Theme().TextMuted)
			}
		})
	}
	tint := colorsFor(c.Theme().Dark).glass
	card := ui.Column(c.Key("media")).FillWidth().Height(float32(s.cardHeight())).Shrink(0).IgnoreInput(control == nil).
		Padding(10, 12).Gap(5).Radius(16).
		Gradient(tint.Alpha(0.48), tint.Alpha(0.19), 155).Border(0.75, tint.Alpha(0.65)).
		InsetShadow(0, 1, 0, 0, tint.Alpha(0.46))
	if large {
		card.Row().Padding(12).Gap(14).AlignItems(ui.Center)
	}
	card.Children(func() {
		if large {
			m.cover(c.Key("cover"), largeMediaCoverSize)
			ui.Column(c.Key("large-details")).Grow(1).MinWidth(0).Height(largeMediaCoverSize).
				Gap(8).Justify(ui.Center).Children(func() {
				metadata()
				ui.Column(c.Key("progress")).FillWidth().Gap(1).Children(progress)
				ui.Row(c.Key("controls")).FillWidth().Height(34).Gap(12).Center().Children(controls)
			})
		} else {
			ui.Row(c.Key("track")).FillWidth().Height(40).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
				m.cover(c.Key("cover"), 40)
				metadata()
				controls()
			})
			progress()
		}
	})
}

func paintMediaBars(p *ui.Painter, r ui.Rect, levels []float32, bandsPerBar int, fraction float32, color ui.Color) {
	bars := float32(len(levels) / bandsPerBar)
	step := r.W / bars
	played, remaining := color.Alpha(0.88), color.Alpha(0.30)
	if color == islandThemes[themeLight].Accent || color == islandThemes[themeDark].Accent {
		remaining = p.Theme().Text.Alpha(0.17)
	}
	for i := 0; i < len(levels); i += bandsPerBar {
		x := float32(i / bandsPerBar)
		level := levels[i]
		for _, band := range levels[i+1 : i+bandsPerBar] {
			level = max(level, band)
		}
		height := float32(5) + 17*level
		fill := remaining
		if (x+0.5)/bars <= fraction {
			fill = played
		}
		p.Fill(ui.Rect{X: r.X + x*step + 1, Y: r.Y + (r.H-height)/2, W: max(0, step-2), H: height}, fill, 2)
	}
}

func mediaTime(seconds float64) string {
	s := int(seconds)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
