package island

import (
	"runtime"
	"time"

	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	compactWidth      = 244
	compactMediaWidth = 200
	compactHeight     = 52
	panelHeaderHeight = 22
	panelWidth        = 428
	windowRadius      = 26
	hotkey            = "CmdOrCtrl+Alt+Space"
)

type island struct {
	win                   *mygo.Window
	hasWindowBackdrop     bool
	tray                  *mygo.Tray
	statePath             string
	previews              map[string]*cachedPreview
	previewJobs           chan previewJob
	previewClock          uint64
	animation             islandAnimation
	media                 islandMedia
	battery               islandBattery
	lyrics                lyricOverlay
	idle                  idleInfo
	face                  idleFaceState
	alert                 capsuleAlert
	expanded              bool
	hiddenForFullscreen   bool
	hiddenByUser          bool
	leaveSince            time.Time
	hotkeyFailed          bool
	registeredHotkey      string
	editingHotkey         bool
	hotkeyProblem         string
	centerX               int
	top                   int
	dock                  edgeDock
	moving                bool
	press                 *capsulePress
	resizing              bool
	windowFramePending    bool
	wakeWindowDisplayLink func()
	shownFrame            islandFrame

	canvas transitionCanvas

	todos              todoComponent
	clipboard          clipboardComponent
	settings           appSettings
	updates            appUpdates
	citySearch         weatherCitySearch
	idleSettingsList   ui.ListState
	settingsPage       string
	settingsWindow     *mygo.Window
	settingsSaveAt     time.Time
	settingsSaveFailed bool
	openAtLogin        bool
	openAtLoginErr     string
	getOpenAtLogin     func() bool
	setOpenAtLogin     func(bool) error
	isSystemDark       func() bool
	listDisplays       func() ([]mygo.Display, mygo.Display)
	showLogs           bool
	logClearErr        string
	versionClicks      int
	versionClickAt     time.Time
	debugUnlocked      bool
}

func (a *island) start() {
	a.applyTheme()
	displays, primary := a.connectedDisplays()
	a.centerX, a.top, a.dock = resolvePlacement(a.settings, displays, primary)
	frame := a.targetFrame()
	a.shownFrame = frame
	a.animation = islandAnimation{from: frame, to: frame, started: time.Now()}
	bounds := a.windowBounds(int(frame.width), int(frame.height))
	options := mygo.WindowOptions{
		Title: "灵动岛", Content: ui.View(a.view),
		X: bounds.X, Y: bounds.Y,
		Width: int(frame.width), Height: int(frame.height), MinWidth: compactHeight, MinHeight: idleCapsuleHeight,
		Frameless: true, Transparent: true, Panel: true, AlwaysOnTop: true,
		Hidden: true, SkipTaskbar: true, DisableResize: true, DisableMaximize: true,
		DisableMinimize: true, DisableFullScreen: true, CornerRadius: windowRadius,
		BackgroundColor: "#00000000", Vibrancy: mygo.VibrancyGlass,
	}
	if runtime.GOOS == "windows" {
		// The Windows backdrop is clipped inside the composition tree.
		options.Vibrancy = mygo.VibrancyNone
		options.DisableShadow = true
	}
	a.win = mygo.NewWindow(options)
	if radius := islandCornerRadius(frame.height); radius < windowRadius {
		setIslandWindowCornerRadius(a.win, radius)
	}
	configureFloatingWindow(a.win)
	a.wakeWindowDisplayLink = startWindowDisplayLink(a)
	a.media.adapter = nowplaying.Adapter()
	a.startPreviews()
	a.win.SetActiveAppearance(true)
	a.hasWindowBackdrop = configureWindowGlass(a.win, windowRadius)
	a.win.OnMove(a.windowMoved)
	a.win.OnBlur(a.clipboardWindowBlurred)
	mygo.Screen.OnDisplaysChanged(a.restorePlacement)
	mygo.Theme.OnUpdated(func() {
		if a.settings.Theme != themeSystem {
			return
		}
		if a.win != nil {
			a.win.Invalidate()
		}
		if a.settingsWindow != nil {
			a.settingsWindow.Invalidate()
		}
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.saveSettings()
		if a.clipboard.stop != nil {
			a.clipboard.stop()
		}
		if a.clipboard.store != nil {
			a.clipboard.store.Close()
			a.clipboard.store = nil
		}
		if a.media.stopWatch != nil {
			a.media.stopWatch()
			a.media.stopWatch = nil
		}
		a.media.stopWave()
		close(a.previewJobs)
		mygo.GlobalShortcut.UnregisterAll()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	if keys := a.settings.toggleHotkey(); a.registerToggleShortcut(keys) != nil {
		a.hotkeyFailed = true
		if a.settingsWindow != nil {
			a.settingsWindow.Invalidate()
		}
	} else {
		a.registeredHotkey = keys
	}
	a.createTray()
	a.syncClipboardRecording()
	a.watchMedia()
	a.watchTodoReminders()
	a.watchFullscreen()
	a.refreshBattery()
	if !a.hiddenForFullscreen {
		a.win.ShowInactive()
	}
}

func (a *island) trayMenu() *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "显示灵动岛", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{Label: "待办事项…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openTodos() }},
	}
	if a.clipboard.Recording {
		items = append(items, &mygo.MenuItem{ID: "clipboard-history", Label: "剪贴板历史…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openClipboard(false) }})
	}
	return mygo.NewMenu(append(items, []*mygo.MenuItem{
		mygo.Separator(),
		{Label: "回到屏幕顶部", Click: func(*mygo.MenuItem, *mygo.Window) { a.recenter() }},
		{Label: "隐藏灵动岛", Click: func(*mygo.MenuItem, *mygo.Window) { a.hide() }},
		mygo.Separator(),
		{Label: "设置…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	}...))
}

func (a *island) createTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID: "dynamic-island", Icon: trayIcon(), IconIsTemplate: true, ToolTip: "灵动岛", Menu: a.trayMenu(),
	})
	if err != nil {
		return
	}
	a.tray = tray
	a.tray.OnClick(a.toggle)
}

func (a *island) compactIdle() bool {
	return !a.media.track.Playing && a.todos.next() == nil
}

func (a *island) toolsPreviewHeight() int {
	height := a.todos.previewHeight()
	if a.clipboard.Recording {
		height = max(height, a.clipboard.previewHeight())
	}
	return height
}

func (a *island) overviewHeight() int {
	return panelHeaderHeight + 20 + a.overviewLeadHeight() + a.toolsPreviewHeight()
}

func (a *island) panelHeight() int {
	if a.todos.open {
		return panelHeaderHeight + 20 + a.todos.detailHeight()
	}
	if a.clipboard.open {
		return panelHeaderHeight + 20 + clipboardPanelHeight
	}
	return a.overviewHeight()
}

func (a *island) setExpanded(expanded bool) {
	if a.moving {
		return
	}
	a.expanded = expanded
	if !expanded {
		a.todos.open = false
		a.todos.draft, a.todos.leaving = nil, nil
		a.clipboard.open, a.clipboard.confirmClear = false, false
		a.clipboard.pendingPaste = nil
	}
	a.media.scrubbing = false
	now := time.Now()
	a.leaveSince = now
	if expanded {
		// Hovering clears this grace; external opens remain reachable.
		a.leaveSince = now.Add(800 * time.Millisecond)
	}
	a.animation.retarget(a.targetFrame(), now)
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *island) hide() {
	a.hiddenByUser = true
	a.hideWindows()
}

func (a *island) hideWindows() {
	if a.moving {
		a.moving = false
		a.animation.resume(time.Now())
	}
	a.lyrics.rows = [3]string{}
	if a.lyrics.win != nil {
		a.lyrics.win.Hide()
	}
	a.face = idleFaceState{}
	a.media.stopWave()
	a.win.Hide()
}

func (a *island) show() {
	a.hiddenByUser = false
	if a.hiddenForFullscreen {
		return
	}
	a.win.Show()
	a.win.Focus()
	a.setExpanded(true)
}

func (a *island) showPanel() {
	a.hiddenByUser = false
	if a.win != nil && !a.win.IsVisible() && !a.hiddenForFullscreen {
		a.show()
		return
	}
	a.setExpanded(true)
	if a.win != nil && !a.hiddenForFullscreen {
		a.win.Update(a.win.Focus)
	}
}

func (a *island) toggle() {
	if a.win.IsVisible() && a.expanded {
		a.setExpanded(false)
		return
	}
	a.show()
}

func (a *island) recenter() {
	displays, primary := a.connectedDisplays()
	target := primary
	if d, ok := findDisplayByID(displays, a.settings.DisplayID); ok {
		target = d
	}
	area := target.WorkArea
	a.dock = edgeDock{}
	a.centerX, a.top = area.X+area.Width/2, area.Y+14
	a.settings.Position = islandPosition{}
	if a.statePath != "" {
		_ = a.settings.save(a.settingsPath())
	}
	if a.win != nil {
		a.applyWindowFrame(a.shownFrame, false)
		a.win.Invalidate()
	}
}

func (a *island) applyWindowFrame(frame islandFrame, animated bool) {
	previous := a.shownFrame
	a.shownFrame = frame
	bounds := a.windowBounds(int(frame.width), int(frame.height))
	nativeBounds := bounds
	usingCanvas := false
	keepExpandedCanvas := a.clipboard.Recording && a.animation.to.open == 1
	if animated && (frame != a.animation.to || keepExpandedCanvas) && a.dock.side == dockNone {
		starting := a.canvas.restore == nil
		if starting && (keepExpandedCanvas || a.animation.from.open != a.animation.to.open || a.animation.from.todo != a.animation.to.todo ||
			a.animation.from.clipboard != a.animation.to.clipboard) {
			a.canvas = startTransitionCanvas(a.win)
			if a.canvas.restore != nil {
				target := a.transitionCanvasBounds(a.canvas.bounds, keepExpandedCanvas)
				if a.canvas.bounds != target {
					// Paint the current island at the larger backing size
					// before starting the visible transition.
					a.canvas.warming = true
					a.shownFrame = previous
					bounds = a.windowBounds(int(previous.width), int(previous.height))
					a.animation.started = a.animation.lastTick
				}
				a.canvas.bounds = target
			}
		}
		if a.canvas.restore != nil && a.canvas.contains(bounds) {
			usingCanvas = true
			nativeBounds = a.canvas.bounds
			if starting || frame.width != previous.width || frame.height != previous.height {
				region := a.canvas.contentRect(bounds)
				a.canvas.reshape(region, nativeBounds.Height)
				a.win.SetInputRegion([]mygo.Rectangle{region})
			}
		}
	}
	var restore func()
	if !usingCanvas {
		restore = a.canvas.restore
		a.canvas = transitionCanvas{}
	}
	if a.win.Bounds() != nativeBounds {
		a.resizing = true
		setIslandWindowBounds(a.win, nativeBounds, animated)
		a.resizing = false
	}
	if restore != nil {
		restore()
		a.win.SetInputRegion(nil)
	}
	if radius := islandCornerRadius(frame.height); !usingCanvas && (restore != nil || radius != islandCornerRadius(previous.height)) {
		setIslandWindowCornerRadius(a.win, radius)
	}
}
