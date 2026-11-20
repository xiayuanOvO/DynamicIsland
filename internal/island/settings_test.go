package island

import (
	"slices"
	"testing"
	"time"

	"dynamicisland/internal/lyrics"
	"github.com/egoist/mygo/ui"
)

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
