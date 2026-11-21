package island

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"dynamicisland/internal/lyrics"
	"dynamicisland/internal/shelf"
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
	statePath := filepath.Join(config, "DynamicIsland", "shelf.json")
	files, missing, loadErr := shelf.Load(statePath)
	a := &island{
		files: files, statePath: statePath, selected: make(map[string]bool),
	}
	if cache, err := os.UserCacheDir(); err == nil {
		a.media.artworkCacheDir = filepath.Join(cache, "DynamicIsland", "artwork")
	}
	a.lyrics.diskCache = lyrics.NewCache(filepath.Dir(statePath))
	a.loadTodos()
	a.loadClipboard()
	a.settings, err = loadSettings(a.settingsPath())
	if err != nil {
		log.Printf("load settings: %v", err)
	}
	if loadErr != nil {
		log.Printf("load shelf: %v", loadErr)
		a.status = "列表读取失败"
		a.statusError = true
	} else if missing > 0 {
		a.status = "已跳过失效文件"
	}

	mygo.App.OnNotificationClick(func(id string) {
		if strings.HasPrefix(id, "todo-") {
			mygo.App.WhenReady(a.openTodos)
		}
	})
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		mygo.App.WhenReady(func() {
			a.show()
			if len(args) > 0 {
				a.addFiles(commandPaths(args, workingDir))
			}
		})
	})
	mygo.App.WhenReady(func() {
		if runtime.GOOS != "darwin" && !mygo.App.IsPackaged() {
			if err := mygo.App.Dock.SetIcon(appIcon); err != nil {
				log.Printf("app icon: %v", err)
			}
		}
		a.start()
		a.startUpdates()
		a.addFiles(commandPaths(os.Args[1:], ""))
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func commandPaths(args []string, dir string) []string {
	paths := make([]string, 0, len(args))
	for _, arg := range args {
		if dir != "" && !filepath.IsAbs(arg) {
			arg = filepath.Join(dir, arg)
		}
		paths = append(paths, arg)
	}
	return paths
}
