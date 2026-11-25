package island

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"dynamicisland/internal/lyrics"
	"github.com/egoist/mygo"
)

func Run(appIcon []byte) {
	mygo.App.SetName("DynamicIsland")
	if mygo.App.Version() == "" {
		mygo.App.SetVersion("0.1.0")
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	statePath := filepath.Join(config, "DynamicIsland", "settings.json")
	initAppLogging(statePath)
	a := &island{statePath: statePath}
	if cache, err := os.UserCacheDir(); err == nil {
		a.media.artworkCacheDir = filepath.Join(cache, "DynamicIsland", "artwork")
	}
	a.lyrics.diskCache = lyrics.NewCache(filepath.Dir(statePath))
	a.loadLyricOffsets()
	a.loadTodos()
	a.loadClipboard()
	a.settings, err = loadSettings(a.settingsPath())
	if err != nil {
		log.Printf("load settings: %v", err)
	}

	mygo.App.OnNotificationClick(func(id string) {
		if strings.HasPrefix(id, "todo-") {
			mygo.App.WhenReady(a.openTodos)
		}
	})
	mygo.App.OnSecondInstance(func([]string, string) {
		mygo.App.WhenReady(a.show)
	})
	mygo.App.WhenReady(func() {
		if runtime.GOOS != "darwin" && !mygo.App.IsPackaged() {
			if err := mygo.App.Dock.SetIcon(appIcon); err != nil {
				log.Printf("app icon: %v", err)
			}
		}
		a.start()
		a.startUpdates()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
