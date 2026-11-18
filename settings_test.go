package main

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSettingsWindowSavesWaveColorChoice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	initial, err := loadSettings(path)
	if err != nil || initial.waveColor(ui.Hex("#b05938")) != accent {
		t.Fatalf("expected default appearance without a settings file: %+v, %v", initial, err)
	}
	a := island{statePath: filepath.Join(dir, "shelf.json"), settings: initial}
	view := ui.NewTester(a.settingsView, settingsWidth, 510)
	view.SetLocale("en")
	custom := initial.Color
	for _, choice := range []struct {
		label string
		mode  int
	}{
		{"跟随海报", waveArtwork},
		{"自定义", waveCustom},
		{"默认", waveDefault},
	} {
		if err := view.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		if choice.mode == waveCustom {
			if err := view.Click("Red"); err != nil {
				t.Fatal(err)
			}
			custom = ui.Hex("#ef4444")
		}
		view.Advance(time.Second)
		saved, err := loadSettings(path)
		if err != nil || saved.Mode != choice.mode || saved.Color != custom {
			t.Fatalf("saved settings after choosing %s: %+v, %v", choice.label, saved, err)
		}
	}
}

func TestArtworkWaveUsesDominantColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 80, 80))
	want := ui.Hex("#336ab6")
	draw.Draw(img, image.Rect(0, 0, 60, 80), image.NewUniform(color.NRGBA{R: want.R, G: want.G, B: want.B, A: 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(60, 0, 72, 80), image.NewUniform(color.NRGBA{R: 200, G: 40, B: 25, A: 255}), image.Point{}, draw.Src)
	settings := appSettings{Mode: waveArtwork, Color: ui.Hex("#af85d2")}
	if got := settings.waveColor(dominantArtworkColor(img)); got != want {
		t.Fatalf("artwork color = %+v, want %+v", got, want)
	}
	if got := settings.waveColor(ui.Transparent); got != accent {
		t.Fatalf("missing artwork should use the default: %+v", got)
	}
	if got := settings.waveColor(white); got.R >= 160 {
		t.Fatalf("light artwork should keep bars visible: %+v", got)
	}
	settings.Mode = waveCustom
	if got := settings.waveColor(want); got != settings.Color {
		t.Fatalf("custom color = %+v, want %+v", got, settings.Color)
	}
}

func TestCustomColorAppliesToBothWaveProgressStates(t *testing.T) {
	view := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(white)
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			paintMediaBars(p, r, []float32{0.5, 0.5}, 1, 0.5, ui.RGB(200, 50, 40))
		})
	}, 20, 24)
	img := view.Image()
	played, upcoming := img.RGBAAt(5, 12), img.RGBAAt(15, 12)
	if played.R <= played.G || upcoming.R <= upcoming.G || played.G >= upcoming.G {
		t.Fatalf("expected red played and lighter red upcoming bars: %+v, %+v", played, upcoming)
	}
}
