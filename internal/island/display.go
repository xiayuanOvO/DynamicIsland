package island

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type islandPosition struct {
	DisplayID int64   `json:"displayId,omitempty"`
	CenterX   float64 `json:"centerX,omitempty"`
	Top       int     `json:"top,omitempty"`
	DockSide  int     `json:"dockSide,omitempty"`
	Saved     bool    `json:"saved,omitempty"`
}

func (a *island) connectedDisplays() ([]mygo.Display, mygo.Display) {
	if a.listDisplays != nil {
		return a.listDisplays()
	}
	if !mygo.App.IsReady() {
		return nil, mygo.Display{}
	}
	return mygo.Screen.Displays(), mygo.Screen.PrimaryDisplay()
}

func findDisplayByID(displays []mygo.Display, id int64) (mygo.Display, bool) {
	if id == 0 {
		return mygo.Display{}, false
	}
	for _, d := range displays {
		if d.ID == id {
			return d, true
		}
	}
	return mygo.Display{}, false
}

func displayName(d mygo.Display, index int, duplicate bool) string {
	name := strings.TrimSpace(d.Label)
	if name == "" {
		name = fmt.Sprintf("显示器 %d", index+1)
	} else if duplicate {
		name = fmt.Sprintf("%s (%d)", name, index+1)
	}
	if d.Primary {
		name += "（主屏）"
	}
	return name
}

func resolvePlacement(settings appSettings, displays []mygo.Display, primary mygo.Display) (centerX, top int, dock edgeDock) {
	preferred := primary
	if d, ok := findDisplayByID(displays, settings.DisplayID); ok {
		preferred = d
	}
	if !settings.Position.Saved {
		area := preferred.WorkArea
		return area.X + area.Width/2, area.Y + 14, edgeDock{}
	}
	target := preferred
	if d, ok := findDisplayByID(displays, settings.Position.DisplayID); ok {
		target = d
	}
	area := target.WorkArea
	ratio := max(0, min(1, settings.Position.CenterX))
	centerX = area.X + int(math.Round(ratio*float64(area.Width)))
	minCenter := area.X + compactHeight/2
	maxCenter := area.X + max(compactHeight/2, area.Width-compactHeight/2)
	centerX = min(max(centerX, minCenter), maxCenter)

	top = area.Y + settings.Position.Top
	maxTop := area.Y + max(0, area.Height-idleCapsuleHeight)
	top = min(max(top, area.Y), maxTop)

	if side := dockSide(settings.Position.DockSide); side == dockLeft || side == dockRight {
		dock = edgeDock{side: side, display: target}
	}
	return centerX, top, dock
}

func (a *island) restorePlacement() {
	displays, primary := a.connectedDisplays()
	a.centerX, a.top, a.dock = resolvePlacement(a.settings, displays, primary)
	if a.win != nil {
		a.applyWindowFrame(a.shownFrame, false)
		a.win.Invalidate()
	}
}

func (a *island) saveCurrentPosition(display mygo.Display) {
	area := display.WorkArea
	ratio := 0.5
	if area.Width > 0 {
		ratio = max(0, min(1, float64(a.centerX-area.X)/float64(area.Width)))
	}
	a.settings.Position = islandPosition{
		DisplayID: display.ID,
		CenterX:   ratio,
		Top:       a.top - area.Y,
		DockSide:  int(a.dock.side),
		Saved:     true,
	}
	if a.statePath != "" {
		_ = a.settings.save(a.settingsPath())
	}
}

func (a *island) displaySettings(c *ui.Context) bool {
	displays, primary := a.connectedDisplays()
	counts := make(map[string]int, len(displays))
	for _, d := range displays {
		counts[strings.TrimSpace(d.Label)]++
	}
	options := make([]string, 0, len(displays)+1)
	ids := make([]int64, 0, len(displays)+1)
	options = append(options, "主显示器")
	ids = append(ids, 0)
	selected := options[0]
	for i, d := range displays {
		label := displayName(d, i, counts[strings.TrimSpace(d.Label)] > 1)
		options = append(options, label)
		ids = append(ids, d.ID)
		if a.settings.DisplayID != 0 && d.ID == a.settings.DisplayID {
			selected = label
		}
	}
	changed := false
	settingsRow(c.Key("display-settings")).Children(func() {
		const label = "显示屏幕"
		ui.Text(c, label).Grow(1).FontSize(13).TextColor(c.Theme().Text).AccessHidden(true)
		if ui.Select(c.Key("display-select"), &selected, options).
			Width(settingsControlWidth).Height(34).FontSize(12).Label(label).Changed() {
			var chosenID int64
			for i, opt := range options {
				if opt == selected {
					chosenID = ids[i]
					break
				}
			}
			a.settings.DisplayID = chosenID
			a.settings.Position = islandPosition{}
			target := primary
			if d, ok := findDisplayByID(displays, chosenID); ok {
				target = d
			}
			area := target.WorkArea
			a.dock = edgeDock{}
			a.centerX, a.top = area.X+area.Width/2, area.Y+14
			if a.win != nil {
				a.win.Update(func() {
					a.applyWindowFrame(a.shownFrame, false)
					a.win.Invalidate()
				})
			}
			changed = true
		}
	})
	return changed
}
