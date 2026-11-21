package island

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	themeLight = iota
	themeDark
)

const (
	settingsBackgroundLight = "#f5f6f8"
	settingsBackgroundDark  = "#202126"
)

var (
	ink    = ui.Hex("#30343b")
	muted  = ui.Hex("#767b84")
	accent = ui.Hex("#657b96")
	white  = ui.RGB(255, 255, 255)
)

func islandTheme(dark bool) *ui.Theme {
	if dark {
		t := ui.DarkTheme()
		t.Text, t.TextMuted, t.Accent = ui.Hex("#e6e9ef"), ui.Hex("#afb5bf"), ui.Hex("#c4cbd5")
		t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#dde1e9"), ui.Hex("#abb4c1"), ui.Hex("#212329")
		t.InactiveAccent = t.Accent
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#2d2e33"), ui.Hex("#33353b"), ui.Hex("#383a3f")
		t.Border, t.Focus, t.Selection = ui.Hex("#43454b"), t.Accent, t.Accent.Alpha(0.33)
		t.Background = ui.Transparent
		t.FontSize, t.Radius = 13, 10
		return t
	}
	t := ui.LightTheme()
	t.Text, t.TextMuted, t.Accent = ink, muted, accent
	t.InactiveAccent = accent
	t.AccentText = white
	t.Background = ui.Transparent
	t.InverseText = white
	t.Surface = white.Alpha(0.32)
	t.SurfaceHover = white.Alpha(0.52)
	t.SurfacePressed = ink.Alpha(0.07)
	t.Border = ink.Alpha(0.10)
	t.Focus = accent.Alpha(0.4)
	t.Scrollbar = ink.Alpha(0.18)
	t.FontSize, t.Radius = 13, 10
	return t
}

type appearanceColors struct {
	glass, eyes, problem, lowBattery, charging ui.Color
	sidebar, settingsCard, mediaPreview, popup ui.Color
}

var lightColors = appearanceColors{
	glass: white, eyes: ui.RGB(48, 48, 48), problem: ui.Hex("#a65b50"),
	lowBattery: ui.Hex("#a63338"), charging: ui.Hex("#237448"),
	sidebar: ui.Hex("#ebeff4"), settingsCard: white.Alpha(0.78),
	mediaPreview: ui.Hex("#e5e8ec"), popup: ui.Hex("#f2f3f5"),
}

var darkColors = appearanceColors{
	glass: ui.Hex("#41434a"), eyes: ui.Hex("#f6f8fb"), problem: ui.Hex("#f3ad9f"),
	lowBattery: ui.Hex("#ff9da3"), charging: ui.Hex("#91d7af"),
	sidebar: ui.Hex("#27282b"), settingsCard: ui.Hex("#28292e"),
	mediaPreview: ui.Hex(settingsBackgroundDark), popup: ui.Hex("#2d2e33"),
}

func colorsFor(dark bool) *appearanceColors {
	if dark {
		return &darkColors
	}
	return &lightColors
}

var islandThemes = [2]*ui.Theme{islandTheme(false), islandTheme(true)}

var settingsThemes = func() [2]*ui.Theme {
	light, dark := *islandThemes[themeLight], *islandThemes[themeDark]
	light.Background = ui.Hex(settingsBackgroundLight)
	light.Accent = ui.Hex("#496782")
	light.SurfacePressed = ui.Hex("#dce5ee")
	dark.Background = ui.Hex(settingsBackgroundDark)
	dark.SurfacePressed = ui.Hex("#51555e")
	return [2]*ui.Theme{&light, &dark}
}()

var panelThemes = func() [2]*ui.Theme {
	light, dark := *islandThemes[themeLight], *islandThemes[themeDark]
	light.Background, dark.Background = lightColors.popup, darkColors.popup
	return [2]*ui.Theme{&light, &dark}
}()

func panelTheme(c *ui.Context) *ui.Theme {
	if c.Theme().Dark {
		return panelThemes[themeDark]
	}
	return panelThemes[themeLight]
}

func (a *island) applyTheme() {
	if a.settings.Theme == themeDark {
		mygo.Theme.SetSource(mygo.ThemeDark)
	} else {
		mygo.Theme.SetSource(mygo.ThemeLight)
	}
}
