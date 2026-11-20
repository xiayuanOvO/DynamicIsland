package island

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"dynamicisland/internal/lyrics"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

func TestClipboardSettingTogglesHistoryEntrypoints(t *testing.T) {
	a := island{expanded: true, files: &shelf.Shelf{}, statePath: filepath.Join(t.TempDir(), "shelf.json")}
	if !a.saveClipboard(clipboardState{Entries: []clipboardEntry{{Text: "样本文字", CopiedAt: time.Now()}}}) {
		t.Fatal(a.clipboard.problem)
	}
	t.Cleanup(func() {
		if a.clipboard.stop != nil {
			a.clipboard.stop()
		}
	})
	settings := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
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
		if reopened.clipboard.Recording != enabled || len(reopened.clipboard.Entries) != 1 || reopened.clipboard.Entries[0].Text != "样本文字" {
			t.Fatal("the setting or existing history was not persisted")
		}
		if enabled {
			menu.Click(menu, nil)
			islandView.Frame()
			if _, ok := islandView.Find("记录剪贴板"); !a.clipboard.open || ok || !islandView.HasText("样本文字") {
				t.Fatal("history should open with the saved entries and keep the switch in settings")
			}
		} else {
			a.openClipboard()
			if a.clipboard.open {
				t.Fatal("history opened while recording was disabled")
			}
		}
	}
}

func TestClipboardSettingsRecoversUnreadableHistory(t *testing.T) {
	a := island{statePath: filepath.Join(t.TempDir(), "shelf.json")}
	if err := os.WriteFile(a.clipboardPath(), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	a.loadClipboard()
	view := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
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
	view := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
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
