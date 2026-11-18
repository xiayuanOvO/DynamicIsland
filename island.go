package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const (
	compactWidth      = 244
	compactMediaWidth = 200
	compactHeight     = 52
	panelWidth        = 428
	windowRadius      = 26
	hotkey            = "CmdOrCtrl+Alt+Space"
	cardColumns       = 3
	cardHeight        = 116
	cardGap           = 8
	maxCardRows       = 2
)

type island struct {
	win          *mygo.Window
	tray         *mygo.Tray
	files        *shelf.Shelf
	statePath    string
	selected     map[string]bool
	previews     map[string]*filePreview
	previewJobs  chan previewJob
	previewClock uint64
	animation    islandAnimation
	media        islandMedia
	lyrics       lyricOverlay
	idle         idleInfo
	expanded     bool
	fullscreen   bool
	hiddenByUser bool
	dragging     bool
	busy         int
	choosing     bool
	leaveSince   time.Time
	blinkAt      time.Time
	status       string
	statusError  bool
	centerX      int
	top          int
	dock         edgeDock
	moving       bool
	press        *capsulePress
	resizing     bool
	resizeQueued bool
	pendingSize  mygo.Size
	width        int
	height       int

	settings           appSettings
	citySearch         weatherCitySearch
	settingsWindow     *mygo.Window
	settingsSaveAt     time.Time
	settingsSaveFailed bool
}

func (a *island) start() {
	mygo.Theme.SetSource(mygo.ThemeLight)
	area := mygo.Screen.PrimaryDisplay().WorkArea
	a.centerX, a.top = area.X+area.Width/2, area.Y+14
	frame := a.targetFrame()
	a.width, a.height = int(frame.width), int(frame.height)
	a.animation = islandAnimation{from: frame, to: frame, started: time.Now()}
	options := mygo.WindowOptions{
		Title: "灵动岛", Content: ui.View(a.view),
		X: a.centerX - a.width/2, Y: a.top,
		Width: a.width, Height: a.height, MinWidth: compactHeight, MinHeight: compactHeight - 4,
		Frameless: true, Transparent: true, Panel: true, AlwaysOnTop: true,
		SkipTaskbar: true, DisableResize: true, DisableMaximize: true,
		DisableMinimize: true, DisableFullScreen: true, CornerRadius: windowRadius,
		BackgroundColor: "#00000000", Vibrancy: mygo.VibrancyGlass,
	}
	if runtime.GOOS == "windows" {
		// DWM composes the Acrylic backdrop at its own window bounds.
		options.Vibrancy = mygo.VibrancyNone
		options.DisableShadow = true
	}
	a.win = mygo.NewWindow(options)
	a.media.adapter = nowplaying.Adapter()
	a.startPreviews()
	a.win.SetActiveAppearance(true)
	a.win.OnResize(func() { roundWindow(a.win, windowRadius) })
	a.win.OnMove(a.windowMoved)
	roundWindow(a.win, windowRadius)
	mygo.Screen.OnDisplaysChanged(a.recenter)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.saveSettings()
		a.media.stopWave()
		close(a.previewJobs)
		mygo.GlobalShortcut.UnregisterAll()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	if err := mygo.GlobalShortcut.Register(hotkey, a.toggle); err != nil {
		log.Printf("global shortcut: %v", err)
		a.message("快捷键被占用", true)
	}
	a.createTray()
	a.watchFullscreen()
}

func (a *island) createTray() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示灵动岛", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		{Label: "添加文件…", Click: func(*mygo.MenuItem, *mygo.Window) { a.show(); a.choose(false) }},
		{Label: "添加文件夹…", Click: func(*mygo.MenuItem, *mygo.Window) { a.show(); a.choose(true) }},
		mygo.Separator(),
		{Label: "回到屏幕顶部", Click: func(*mygo.MenuItem, *mygo.Window) { a.recenter() }},
		{Label: "隐藏灵动岛", Click: func(*mygo.MenuItem, *mygo.Window) { a.hide() }},
		mygo.Separator(),
		{Label: "设置…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
		{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	icon := trayIcon()
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID: "dynamic-island", Icon: icon, IconIsTemplate: true, ToolTip: "灵动岛", Menu: menu,
	})
	if err != nil {
		log.Printf("tray: %v", err)
		a.message("托盘创建失败", true)
		return
	}
	a.tray = tray
	a.tray.OnClick(a.toggle)
	if err := a.win.SetIcon(icon); err != nil {
		log.Printf("window icon: %v", err)
	}
}

func (a *island) compactIdle() bool {
	return len(a.files.Items) == 0 && !a.media.track.Playing
}

func (a *island) emptyIsland() bool {
	return a.compactIdle() && a.media.track.Title == ""
}

func (a *island) panelHeights() (height, gridHeight int) {
	hasFiles := len(a.files.Items) > 0
	height = compactHeight + 10 + 20
	if hasFiles {
		height = 225
		rows := min((len(a.files.Items)+cardColumns-1)/cardColumns, maxCardRows)
		gridHeight = rows*cardHeight + max(rows-1, 0)*cardGap + 8
	} else if a.status != "" {
		height += 20
	}
	if a.media.track.Title != "" {
		height += mediaCardHeight
		if hasFiles || a.status != "" {
			height += 14
		}
	}
	if a.dock.side != dockNone {
		gridHeight = min(gridHeight, max(0, a.dock.display.WorkArea.Height-height))
	}
	return height + gridHeight, gridHeight
}

func (a *island) setExpanded(expanded bool) {
	if a.dragging || a.moving {
		return
	}
	if expanded && a.emptyIsland() && a.status == "" {
		return
	}
	a.expanded = expanded
	a.media.scrubbing = false
	if expanded {
		a.media.refreshAt = time.Time{}
	}
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
	a.idle.anchor = mygo.Rectangle{}
	if a.idle.win != nil {
		a.idle.win.Hide()
	}
	a.media.stopWave()
	a.win.Hide()
}

func (a *island) show() {
	a.hiddenByUser = false
	if a.fullscreen {
		return
	}
	a.win.Show()
	a.win.Focus()
	a.setExpanded(true)
}

func (a *island) toggle() {
	if a.win.IsVisible() && a.expanded {
		a.setExpanded(false)
		return
	}
	a.show()
}

func (a *island) recenter() {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	a.dock = edgeDock{}
	a.centerX, a.top = area.X+area.Width/2, area.Y+14
	a.resize(a.width, a.height)
	a.win.Invalidate()
}

func (a *island) resize(width, height int) {
	a.pendingSize = mygo.Size{Width: width, Height: height}
	if a.resizeQueued {
		return
	}
	a.resizeQueued = true
	// Resizing synchronously during a view build re-enters the renderer.
	a.win.Update(func() {
		a.resizeQueued = false
		if a.moving {
			return
		}
		size := a.pendingSize
		a.resizing = true
		a.width, a.height = size.Width, size.Height
		a.win.SetBounds(a.windowBounds(size.Width, size.Height))
		a.resizing = false
	})
}

func (a *island) message(message string, failed bool) {
	a.status, a.statusError = message, failed
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *island) save() {
	if err := a.files.Save(a.statePath); err != nil {
		log.Printf("save shelf: %v", err)
		a.message("列表保存失败", true)
	}
}

func (a *island) addFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	a.busy++
	a.message("加载中…", false)
	go func() {
		items, rejected := shelf.Inspect(paths)
		a.win.Update(func() {
			a.busy--
			report := a.files.Add(items)
			for _, item := range items {
				delete(a.previews, shelf.Key(item.Path))
			}
			report.Rejected = rejected
			parts := make([]string, 0, 3)
			if report.Added > 0 {
				parts = append(parts, fmt.Sprintf("已暂存 %d 项", report.Added))
			}
			if report.Duplicates > 0 {
				parts = append(parts, fmt.Sprintf("%d 项已存在", report.Duplicates))
			}
			if rejected > 0 {
				parts = append(parts, fmt.Sprintf("%d 项不可访问", rejected))
			}
			a.message(strings.Join(parts, " · "), rejected > 0)
			if report.Added > 0 {
				a.save()
			}
			a.setExpanded(true)
		})
	}()
}

func (a *island) choose(directory bool) {
	if a.choosing {
		return
	}
	a.choosing = true
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "添加文件", Multiple: true, Directory: directory,
		})
		a.win.Update(func() {
			a.choosing = false
			a.leaveSince = time.Now()
			if err != nil {
				a.message("打开文件选择器失败", true)
				log.Printf("file dialog: %v", err)
				return
			}
			a.addFiles(paths)
		})
	}()
}

func (a *island) selection() []string {
	paths := make([]string, 0, len(a.selected))
	for _, item := range a.files.Items {
		if a.selected[shelf.Key(item.Path)] {
			paths = append(paths, item.Path)
		}
	}
	return paths
}

func (a *island) outgoingPaths(path string) []string {
	paths := a.selection()
	if path != "" && !a.selected[shelf.Key(path)] {
		return []string{path}
	}
	if len(paths) == 0 {
		return a.files.Paths()
	}
	return paths
}

func (a *island) dragData(path string) transfer.Data {
	paths, missing := available(a.outgoingPaths(path))
	if missing > 0 {
		a.message(fmt.Sprintf("%d 项不可访问", missing), true)
	}
	data, err := transfer.FileData(paths...)
	if err != nil {
		a.message("拖放数据创建失败", true)
		return transfer.Data{}
	}
	a.dragging = len(paths) > 0
	if a.dragging {
		a.animation.pause(time.Now())
	}
	return data
}

func (a *island) dragDone(result transfer.Result) {
	a.dragging = false
	a.leaveSince = time.Now()
	a.animation.resume(a.leaveSince)
	switch {
	case result.Err != nil:
		log.Printf("file drag: %v", result.Err)
		a.message("拖放失败", true)
	case result.Canceled:
		a.message("拖放已取消", false)
	case result.Operation == transfer.Copy:
		a.message("拖放完成", false)
	}
	a.win.Invalidate()
}

func available(paths []string) ([]string, int) {
	valid := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			valid = append(valid, path)
		}
	}
	return valid, len(paths) - len(valid)
}

func (a *island) copyFiles(paths []string) {
	paths, missing := available(paths)
	if len(paths) == 0 {
		a.message("文件不可访问", true)
		return
	}
	if err := mygo.Clipboard.WriteFiles(paths...); err != nil {
		a.message("复制失败", true)
		return
	}
	if err := mygo.Clipboard.Flush(); err != nil {
		log.Printf("flush clipboard: %v", err)
	}
	message := fmt.Sprintf("已复制 %d 项", len(paths))
	if missing > 0 {
		message += fmt.Sprintf("（跳过 %d 项）", missing)
	}
	a.message(message, false)
}

func (a *island) removeFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	a.files.Remove(paths)
	for _, path := range paths {
		delete(a.selected, shelf.Key(path))
		delete(a.previews, shelf.Key(path))
	}
	a.message(fmt.Sprintf("已移除 %d 项", len(paths)), false)
	a.save()
}
