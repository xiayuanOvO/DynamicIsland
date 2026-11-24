package island

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"dynamicisland/internal/lyrics"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestSettingsSidebarNavigationAndSave(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	const height = 360
	view := ui.NewTester(a.settingsView, settingsWidth, height)
	if a.settingsPage != settingsPageIdle || !view.HasText("显示表情") || view.HasText("音乐卡片预览") {
		t.Fatalf("settings should initially show the idle page: %v", view.Texts())
	}
	view.Key(0, ui.KeyTab)
	view.Key(0, ui.KeyDown)
	if a.settingsPage != settingsPageMedia || !view.HasText("音乐卡片预览") || view.HasText("显示表情") {
		t.Fatalf("Down should open the media page from the sidebar: %v", view.Texts())
	}
	sidebar, found := view.Find("设置分类")
	if !found {
		t.Fatal("the settings category sidebar is missing")
	}
	view.Scroll(settingsSidebarWidth+100, height-30, 0, 460)
	view.Advance(300 * time.Millisecond)
	if moved, ok := view.Find("设置分类"); !ok || moved != sidebar || !view.HasText("律动配色模式") {
		t.Fatalf("scrolling the media page should leave the sidebar fixed: %v", view.Texts())
	}
	for _, page := range []struct{ label, id, setting string }{
		{"剪贴板", settingsPageClipboard, "记录剪贴板"},
		{"应用", settingsPageApp, "检查更新"},
		{"空闲", settingsPageIdle, "显示表情"},
	} {
		if err := view.Click(page.label); err != nil {
			t.Fatal(err)
		}
		if a.settingsPage != page.id || !view.HasText(page.setting) {
			t.Fatalf("clicking %s did not open its settings: %v", page.label, view.Texts())
		}
	}
	if err := view.Click("显示表情"); err != nil {
		t.Fatal(err)
	}
	if err := view.Click("应用"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	saved, err := loadSettings(a.settingsPath())
	if err != nil || saved.showsIdleItem(idleItemFace) || a.settingsPage != settingsPageApp {
		t.Fatalf("switching pages dropped a pending setting change: %+v, %v", saved.idleItems(), err)
	}
}

func TestThemeSettingCanBeChangedAndSaved(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	for _, data := range []string{`{"hideLyrics":true}`, `{"hideLyrics":true,"theme":99}`} {
		if err := os.WriteFile(a.settingsPath(), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		settings, err := loadSettings(a.settingsPath())
		if err != nil || settings.Theme != themeLight || !settings.HideLyrics {
			t.Fatalf("expected to fall back to light and keep the other settings, got %+v, %v", settings, err)
		}
		a.settings = settings
	}

	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	view.UseScreenReader()
	if err := view.Click("主题"); err != nil {
		t.Fatal(err)
	}
	if _, ok := view.Find("外观"); a.settingsPage != settingsPageTheme || !ok {
		t.Fatalf("the Theme page should expose the appearance control: %v", view.Texts())
	}
	for range len(settingsPages) + 1 {
		if view.Focused("浅色") {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused("浅色") {
		t.Fatal("the appearance control should be reachable by Tab")
	}
	for _, step := range []struct {
		key        ui.Key
		want       int
		background ui.Color
	}{
		{ui.KeyRight, themeDark, ui.Hex(settingsBackgroundDark)},
		{ui.KeyRight, themeSystem, ui.Hex(settingsBackgroundLight)},
		{ui.KeyLeft, themeDark, ui.Hex(settingsBackgroundDark)},
		{ui.KeyLeft, themeLight, ui.Hex(settingsBackgroundLight)},
	} {
		a.isSystemDark = func() bool { return false }
		view.Key(0, step.key)
		view.Advance(time.Second / 30)
		background := view.Image().RGBAAt(settingsWidth-8, settingsHeight-8)
		if a.settings.Theme != step.want || background.R != step.background.R ||
			background.G != step.background.G || background.B != step.background.B {
			t.Fatalf("switching themes should repaint immediately, got theme %d and background %v", a.settings.Theme, background)
		}
		view.Advance(time.Second)
		saved, err := loadSettings(a.settingsPath())
		if err != nil || saved.Theme != step.want || !saved.HideLyrics {
			t.Fatalf("switching themes should save the new choice and keep other settings, got %+v, %v", saved, err)
		}
	}

	// Verify system theme reacts when the OS switches to dark mode.
	a.settings.Theme = themeSystem
	a.isSystemDark = func() bool { return true }
	view.Frame()
	darkBg := ui.Hex(settingsBackgroundDark)
	background := view.Image().RGBAAt(settingsWidth-8, settingsHeight-8)
	if background.R != darkBg.R || background.G != darkBg.G || background.B != darkBg.B {
		t.Fatalf("system theme should follow dark OS appearance, got background %v", background)
	}
}

func TestDarkSettingsSelectedSegmentStandsOut(t *testing.T) {
	a := island{
		statePath:    filepath.Join(t.TempDir(), "shelf.json"),
		settingsPage: settingsPageMedia,
		settings:     appSettings{Theme: themeDark},
	}
	const height = 590
	view := ui.NewTester(a.settingsView, settingsWidth, height)
	for _, pair := range [][2]string{{"上移", "淡入"}, {"淡入", "上移"}} {
		selected, unselected := pair[0], pair[1]
		if err := view.Click(selected); err != nil {
			t.Fatal(err)
		}
		view.Move(settingsWidth-2, height-2)
		chosen, chosenOK := view.Find(selected)
		other, otherOK := view.Find(unselected)
		if !chosenOK || !otherOK {
			t.Fatal("the lyric transition options should be visible")
		}
		frame := view.Image()
		// Sample the fill just above each label, outside the text.
		on := frame.RGBAAt(int(chosen.X+chosen.W/2), int(chosen.Y)-3)
		off := frame.RGBAAt(int(other.X+other.W/2), int(other.Y)-3)
		if int(on.R)-int(off.R) < 30 {
			t.Errorf("selected segment %q should stand out from the track, got %v against %v", selected, on, off)
		}
	}
}

func TestFullscreenHidingSettingCanBeChangedAndSaved(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	for _, existing := range []bool{false, true} {
		if existing {
			if err := os.WriteFile(a.settingsPath(), []byte(`{"hideLyrics":true}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		settings, err := loadSettings(a.settingsPath())
		if err != nil || !settings.HideInFullscreen {
			t.Fatalf("fullscreen hiding should default to on, got %v, %v", settings.HideInFullscreen, err)
		}
		a.settings = settings
	}

	const label = "全屏时隐藏灵动岛"
	a.settingsPage = settingsPageApp
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	view.UseScreenReader()
	if _, ok := view.Find(label); ok != supportsFullscreenHiding {
		t.Fatalf("fullscreen setting availability = %v, want %v", ok, supportsFullscreenHiding)
	}
	if !supportsFullscreenHiding {
		return
	}
	for range 20 {
		if view.Focused(label) {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused(label) {
		t.Fatal("the fullscreen hiding switch was not reachable by Tab")
	}
	for _, enabled := range []bool{false, true} {
		view.Key(0, ui.KeySpace)
		view.Advance(time.Second)
		saved, err := loadSettings(a.settingsPath())
		if err != nil || a.settings.HideInFullscreen != enabled || saved.HideInFullscreen != enabled || !saved.HideLyrics {
			t.Fatalf("switching fullscreen hiding to %v did not preserve the expected settings: %+v, %v", enabled, saved, err)
		}
	}
}

func TestClipboardSettingTogglesHistoryEntrypoints(t *testing.T) {
	a := newTestClipboard(t, false, "样本文字")
	a.expanded = true
	t.Cleanup(func() {
		if a.clipboard.stop != nil {
			a.clipboard.stop()
		}
	})
	a.settingsPage = settingsPageClipboard
	settings := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	islandView := ui.NewTester(func(c *ui.Context) { a.body(c, a.targetFrame()) }, panelWidth, clipboardPanelHeight+20)
	for _, enabled := range []bool{false, true, false, true} {
		if a.clipboard.Recording != enabled {
			for range 6 {
				if settings.Focused("记录剪贴板") {
					break
				}
				settings.Key(0, ui.KeyTab)
			}
			if !settings.Focused("记录剪贴板") {
				t.Fatal("the recording switch was not reachable in settings")
			}
			settings.Key(0, ui.KeySpace)
		}
		islandView.Frame()
		_, card := islandView.Find("查看剪贴板历史")
		menu := a.trayMenu().ItemByID("clipboard-history")
		if a.clipboard.Recording != enabled || card != enabled || (menu != nil) != enabled || (a.clipboard.stop != nil) != enabled {
			t.Fatalf("setting %t did not update recording, island, and tray together", enabled)
		}
		reopened := island{statePath: a.statePath}
		reopened.loadClipboard()
		entries := readTestClipboardEntries(t, reopened.clipboard.store, time.Now())
		reopened.clipboard.store.Close()
		if reopened.clipboard.Recording != enabled || len(entries) != 1 || entries[0].Preview != "样本文字" {
			t.Fatal("the setting or existing history was not persisted")
		}
		if enabled {
			menu.Click(menu, nil)
			islandView.Frame()
			if _, ok := islandView.Find("记录剪贴板"); !a.clipboard.open || ok || !islandView.HasText("样本文字") {
				t.Fatal("history should open with the saved entries and keep the switch in settings")
			}
		} else {
			a.openClipboard(true)
			if a.clipboard.open {
				t.Fatal("history opened while recording was disabled")
			}
		}
	}
}

func TestClipboardSettingsRecoversUnreadableHistory(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	if err := os.WriteFile(a.clipboardPath(), bytes.Repeat([]byte("{"), 4096), 0600); err != nil {
		t.Fatal(err)
	}
	a.loadClipboard()
	a.settingsPage = settingsPageClipboard
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	if !view.HasText("剪贴板历史读取失败") {
		t.Fatal("settings should report the unreadable history")
	}
	if err := view.Click("清空历史…"); err != nil {
		t.Fatal(err)
	}
	if !view.Focused("取消") {
		t.Fatal("the non-destructive action should be focused")
	}
	if err := view.Click("清空历史"); err != nil {
		t.Fatal(err)
	}
	if a.clipboard.loadFailed || a.clipboard.Recording || !view.HasText("记录剪贴板") {
		t.Fatal("clearing unreadable history should restore the recording switch")
	}
	a.clipboard.store.Close()
}

func TestClipboardShortcutCanBeRecordedAndSavedFromSettings(t *testing.T) {
	a := newTestClipboard(t, false)
	a.settingsPage = settingsPageClipboard
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	defaultLabel, newLabel := "Alt + Shift + V", "Ctrl + Shift + K"
	if runtime.GOOS == "darwin" {
		defaultLabel, newLabel = "Option + Shift + V", "Control + Shift + K"
	}
	if err := view.Click("修改剪贴板快捷键 " + defaultLabel); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyV)
	if a.clipboard.hotkey() != defaultClipboardHotkey || !view.HasText("请使用组合键或功能键") {
		t.Fatal("bare typing keys should leave the current global shortcut intact")
	}
	view.Key(ui.Cmd, ui.KeyV)
	if a.clipboard.hotkey() != defaultClipboardHotkey || !view.HasText("此组合用于系统粘贴") {
		t.Fatal("the normal paste combination should stay available to the target app")
	}
	view.Key(ui.Ctrl|ui.Shift, ui.KeyK)
	if a.clipboard.editingShortcut || a.clipboard.hotkey() != "Ctrl+Shift+K" || !view.HasText(newLabel) {
		t.Fatal("recording a combination should update the button and finish editing")
	}
	reopened := island{statePath: a.statePath}
	reopened.loadClipboard()
	defer reopened.clipboard.store.Close()
	if reopened.clipboard.hotkey() != "Ctrl+Shift+K" || reopened.clipboard.Recording {
		t.Fatal("the customized shortcut should survive restart while recording is disabled")
	}
	if err := view.Click("修改剪贴板快捷键 " + newLabel); err != nil {
		t.Fatal(err)
	}
	if _, err := a.clipboard.store.Exec(`CREATE TRIGGER reject_setting BEFORE UPDATE ON settings
		BEGIN SELECT RAISE(ABORT, 'test settings write failed'); END`); err != nil {
		t.Fatal(err)
	}
	view.Key(ui.Alt|ui.Shift, ui.KeyJ)
	if a.clipboard.hotkey() != "Ctrl+Shift+K" || !view.HasText("快捷键保存失败") {
		t.Fatal("a failed write should report the error and keep the current shortcut")
	}
	view.Key(0, ui.KeyEscape)
	if a.clipboard.editingShortcut || a.clipboard.hotkey() != "Ctrl+Shift+K" {
		t.Fatal("Escape should cancel the shortcut edit")
	}
}

func TestUpdateActivityUsesOneRingWithoutText(t *testing.T) {
	a := island{updates: appUpdates{phase: updateChecking, interactive: true}}
	view := ui.NewTester(a.updateControl, 100, 46)
	firstFrame := slices.Clone(view.Image().Pix)
	view.Advance(150 * time.Millisecond)
	if slices.Equal(firstFrame, view.Image().Pix) {
		t.Fatal("the checking ring did not rotate")
	}
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	checking := slices.Clone(view.Image().Pix)
	if labels := view.Texts(); !slices.Equal(labels, []string{"正在检查更新"}) {
		t.Fatalf("unexpected labels while checking: %v", labels)
	}

	a.updates.phase, a.updates.percent = updateDownloading, 25
	view.Frame()
	if labels := view.Texts(); !slices.Equal(labels, []string{"更新下载进度"}) {
		t.Fatalf("unexpected labels while downloading: %v", labels)
	}
	if !slices.Equal(checking, view.Image().Pix) {
		t.Fatal("checking and downloading painted different quarter rings")
	}

	a.updates.percent = 100
	view.Frame()
	if labels := view.Texts(); !slices.Equal(labels, []string{"正在安装更新"}) {
		t.Fatalf("unexpected labels while installing: %v", labels)
	}
	if slices.Equal(checking, view.Image().Pix) {
		t.Fatal("the ring did not change at full progress")
	}
}

func TestLyricTransitionSettingCanBeChangedAndSaved(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	if err := os.WriteFile(a.settingsPath(), []byte(`{"lyricTransition":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := loadSettings(a.settingsPath())
	if err != nil || settings.LyricTransition != lyricTransitionSlide {
		t.Fatalf("saved unknown effect should fall back to slide, got %d, %v", settings.LyricTransition, err)
	}
	a.settings = settings
	a.settingsPage = settingsPageMedia
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	view.UseScreenReader()
	for range 12 {
		if view.Focused("上移") {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused("上移") {
		t.Fatal("the lyric transition setting was not reachable by Tab")
	}
	for style, label := range []string{"淡入", "缩放", "关闭"} {
		view.Key(0, ui.KeyRight)
		if !view.Focused(label) || a.settings.LyricTransition != style+1 {
			t.Fatalf("Right should choose %s, got %d", label, a.settings.LyricTransition)
		}
	}
	view.Advance(time.Second)
	saved, err := loadSettings(a.settingsPath())
	if err != nil || saved.LyricTransition != lyricTransitionNone {
		t.Fatalf("transition setting did not persist, got %d, %v", saved.LyricTransition, err)
	}
	if err := view.Click("歌词显示"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.HideLyrics || view.HasText("歌词过渡") {
		t.Fatal("the effect setting should be hidden when lyrics are off")
	}
	if err := view.Click("歌词显示"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("歌词过渡") || a.settings.LyricTransition != lyricTransitionNone {
		t.Fatal("showing lyrics should restore the effect setting and its selection")
	}
}

func TestLyricStyleSettingsCanBeChangedAndSaved(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	a.settingsPage = settingsPageMedia
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	view.SetLocale("en-US")
	for range 12 {
		if view.Focused("歌词字号") {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused("歌词字号") {
		t.Fatal("the lyric size slider was not reachable by Tab")
	}
	view.Key(0, ui.KeyRight)
	if got := a.settings.LyricStyle.fontSize(); got != lyricDefaultFontSize+1 {
		t.Fatalf("Right should increase the font size by one, got %d", got)
	}
	view.Key(0, ui.KeyEnd)
	view.Key(0, ui.KeyTab)
	if !view.Focused("歌词高亮颜色") {
		t.Fatal("the lyric color picker was not reachable by Tab")
	}
	view.Key(0, ui.KeyEnter)
	if err := view.Click("Green"); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyEscape)
	view.Advance(time.Second)
	saved, err := loadSettings(a.settingsPath())
	green := ui.Hex("#22c55e")
	if err != nil || saved.LyricStyle.fontSize() != lyricMaxFontSize || saved.LyricStyle.highlightColor() != green {
		t.Fatalf("lyric style did not persist, got size %d and color %v, %v", saved.LyricStyle.fontSize(), saved.LyricStyle.highlightColor(), err)
	}
	if err := view.Click("恢复默认歌词颜色"); err != nil {
		t.Fatal(err)
	}
	view.Advance(time.Second)
	saved, err = loadSettings(a.settingsPath())
	if err != nil || saved.LyricStyle.highlightColor() != white || saved.LyricStyle.fontSize() != lyricMaxFontSize {
		t.Fatalf("reset should restore white and keep the chosen size, got %+v, %v", saved.LyricStyle, err)
	}
}

func TestClearLyricCacheFromSettings(t *testing.T) {
	cache := lyrics.NewCache(t.TempDir())
	song := lyrics.Song{Title: "夜行列车", Artist: "林海风", Duration: 226}
	other := lyrics.Song{Title: "灯塔", Artist: "林海风", Duration: 200}
	lines := lyrics.Lines{{At: 12.5, Text: "窗外亮起一盏灯"}}
	if err := cache.Save(song, lines); err != nil {
		t.Fatal(err)
	}
	a := island{lyrics: lyricOverlay{diskCache: cache, cache: map[lyrics.Song]lyrics.Lines{song: lines, other: lines}, song: song}}
	a.refreshLyricCacheSize()
	a.settingsPage = settingsPageMedia
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	view.UseScreenReader()
	if !view.HasText("< 0.01 MB") {
		t.Fatalf("expected small cache size in MB: %v", view.Texts())
	}
	for range 12 {
		if view.Focused("清理缓存") {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused("清理缓存") {
		t.Fatal("clear button was not reachable by Tab")
	}
	view.Key(0, ui.KeyEnter)
	view.Frame()
	if !view.HasText("0.00 MB") || a.lyrics.cacheVersion != 1 {
		t.Fatalf("clear action did not update the size: %v", view.Texts())
	}
	if announced := view.Announcements(); !slices.Contains(announced, "歌词缓存已清理") {
		t.Fatalf("clear result was not announced: %v", announced)
	}
	if got := a.lyrics.cache[song]; len(got) != len(lines) || len(a.lyrics.cache) != 1 {
		t.Fatalf("clearing did not preserve only the current lyrics: %v", a.lyrics.cache)
	}
	if size, err := cache.Size(); err != nil || size != 0 {
		t.Fatalf("settings left %d bytes of cache: %v", size, err)
	}
}

func TestGlobalHotkeyConflictShowsInAppSettingsNotPanel(t *testing.T) {
	a := island{
		files:        &shelf.Shelf{},
		selected:     make(map[string]bool),
		expanded:     true,
		hotkeyFailed: true,
		settingsPage: settingsPageApp,
	}
	a.shownFrame = a.targetFrame()
	panel := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Clip().Children(func() {
			ui.Box(c).Height(panelHeaderHeight).Shrink(0)
			a.body(c, a.shownFrame)
		})
	}, panelWidth, int(a.shownFrame.height))
	if panel.HasText("快捷键被占用") || panel.HasText("剪贴板无文件") {
		t.Fatalf("panel should not display status banner texts: %v", panel.Texts())
	}

	settings := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	if !settings.HasText("展开 / 收起快捷键") || !settings.HasText(formatHotkey(hotkey)) || !settings.HasText("快捷键被占用") {
		t.Fatalf("app settings should show global hotkey conflict: %v", settings.Texts())
	}

	a.hotkeyFailed = false
	settings.Frame()
	if settings.HasText("快捷键被占用") {
		t.Fatalf("app settings should hide conflict text when hotkey is available: %v", settings.Texts())
	}
}

func TestToggleShortcutCanBeRecordedAndSavedFromSettings(t *testing.T) {
	a := island{
		statePath:    filepath.Join(t.TempDir(), "shelf.json"),
		hotkeyFailed: true,
		settingsPage: settingsPageApp,
	}
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	defaultLabel := formatHotkey(hotkey)
	newLabel := formatHotkey("Ctrl+Shift+Space")
	if err := view.Click("修改展开 / 收起快捷键 " + defaultLabel); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeySpace)
	if a.settings.toggleHotkey() != hotkey || !view.HasText("请使用组合键或功能键") {
		t.Fatal("bare typing keys should leave the current toggle shortcut intact")
	}
	view.Key(ui.Ctrl|ui.Shift, ui.KeySpace)
	if a.editingHotkey || a.hotkeyFailed || a.settings.toggleHotkey() != "Ctrl+Shift+Space" || !view.HasText(newLabel) || view.HasText("快捷键被占用") {
		t.Fatalf("recording a combination should update the button, clear conflict state, and finish editing: %v", view.Texts())
	}
	saved, err := loadSettings(a.settingsPath())
	if err != nil || saved.toggleHotkey() != "Ctrl+Shift+Space" {
		t.Fatalf("the customized toggle shortcut should persist across restarts: %+v, %v", saved, err)
	}
	if err := view.Click("修改展开 / 收起快捷键 " + newLabel); err != nil {
		t.Fatal(err)
	}
	view.Key(0, ui.KeyEscape)
	if a.editingHotkey || a.settings.toggleHotkey() != "Ctrl+Shift+Space" {
		t.Fatal("Escape should cancel the toggle shortcut edit")
	}
}

func TestOpenAtLoginSwitchUpdatesStateAndReportsFailure(t *testing.T) {
	systemEnabled := false
	failNext := false
	a := island{
		statePath:    filepath.Join(t.TempDir(), "shelf.json"),
		settingsPage: settingsPageApp,
		getOpenAtLogin: func() bool {
			return systemEnabled
		},
		setOpenAtLogin: func(enabled bool) error {
			if failNext {
				return errors.New("test login item error")
			}
			systemEnabled = enabled
			return nil
		},
	}
	a.refreshOpenAtLogin()

	const label = "开机启动"
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	view.UseScreenReader()
	for range 20 {
		if view.Focused(label) {
			break
		}
		view.Key(0, ui.KeyTab)
	}
	if !view.Focused(label) {
		t.Fatal("the launch-at-login switch was not reachable by Tab")
	}
	for _, want := range []bool{true, false} {
		view.Key(0, ui.KeySpace)
		if a.openAtLogin != want || systemEnabled != want || view.HasText("设置开机启动失败") {
			t.Fatalf("toggling launch-at-login to %v failed: switch=%v system=%v texts=%v", want, a.openAtLogin, systemEnabled, view.Texts())
		}
	}

	failNext = true
	view.Key(0, ui.KeySpace)
	if a.openAtLogin || systemEnabled || !view.HasText("设置开机启动失败") {
		t.Fatalf("a failed update should revert the switch and report the error: switch=%v system=%v texts=%v", a.openAtLogin, systemEnabled, view.Texts())
	}
}

func TestWeatherCacheAndOpenMeteo14DayForecast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"current":{"temperature_2m":21.6,"weather_code":0},
			"daily":{
				"time":["2026-11-24","2026-11-25"],
				"weather_code":[0,61],
				"temperature_2m_max":[22.1,18.4]
			}
		}`)
	}))
	t.Cleanup(server.Close)

	cache, err := fetchOpenMeteoWeather(context.Background(), server.URL, 39.9, 116.4, "1:39.90000,116.40000")
	if err != nil {
		t.Fatal(err)
	}
	if cache.Current.Temperature != 22 || cache.Current.Condition != "晴" || len(cache.Days) != 2 || !cache.Days[1].Rain {
		t.Fatalf("unexpected open-meteo cache: %+v", cache)
	}
	path := filepath.Join(t.TempDir(), "weather-cache.json")
	if err := saveWeatherCache(path, cache); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadWeatherCache(path)
	if err != nil || !weatherDataEqual(cache, loaded) {
		t.Fatalf("weather cache should round-trip identically: %+v, %v", loaded, err)
	}
	// When current cache is older than 2 hours, weatherFromCache should fall back to matching day in Days.
	nextDay := time.Date(2026, time.November, 25, 10, 0, 0, 0, time.Local)
	loaded.UpdatedAt = nextDay.Add(-12 * time.Hour)
	w, _, ok := weatherFromCache(loaded, nextDay)
	if !ok || w.temperature != 18 || w.condition != "雨" {
		t.Fatalf("expected fallback to cached daily forecast: %+v, ok=%v", w, ok)
	}
}

func TestAppLogViewerAndClearInSettings(t *testing.T) {
	prev := appLogs
	appLogs = &appLogStore{}
	t.Cleanup(func() { appLogs = prev })
	appLogs.init(filepath.Join(t.TempDir(), "app.log"))
	_, _ = appLogs.Write([]byte("2026/11/24 weather wttr.in failed: timeout\n"))

	a := island{
		statePath:    filepath.Join(t.TempDir(), "shelf.json"),
		settingsPage: settingsPageApp,
	}
	view := ui.NewTester(a.settingsView, settingsWidth, settingsHeight)
	if err := view.Click("查看日志"); err != nil {
		t.Fatal(err)
	}
	if !a.showLogs || !view.HasText("2026/11/24 weather wttr.in failed: timeout") {
		t.Fatalf("log viewer should display recorded log lines: %v", view.Texts())
	}
	if err := view.Click("清理日志"); err != nil {
		t.Fatal(err)
	}
	if appLogs.count() != 0 || !view.HasText("暂无日志") {
		t.Fatalf("clearing logs should empty the store and show empty state: count=%d texts=%v", appLogs.count(), view.Texts())
	}
}
