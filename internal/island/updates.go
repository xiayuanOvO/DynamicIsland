package island

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type updatePhase uint8

const (
	updateIdle updatePhase = iota
	updateChecking
	updateOffering
	updateDownloading
	updateLatest
	updateReady

	updateInterval = 24 * time.Hour
)

// Keep the preferences saved by the MyGo updater in earlier releases.
type updatePreferences struct {
	AutomaticChecks    *bool     `json:"automaticChecks,omitempty"`
	AutomaticDownloads bool      `json:"automaticDownloads,omitempty"`
	SkippedVersion     string    `json:"skippedVersion,omitempty"`
	LastCheck          time.Time `json:"lastCheck,omitzero"`
}

type appUpdates struct {
	phase        updatePhase
	percent      int
	interactive  bool
	automatic    bool
	announcement string
	preferences  updatePreferences
	ctx          context.Context
	timer        *time.Timer
}

func (a *island) startUpdates() {
	u := &a.updates
	path := filepath.Join(filepath.Dir(a.statePath), "updater.json")
	data, err := os.ReadFile(path)
	if err == nil {
		var preferences updatePreferences
		if err := json.Unmarshal(data, &preferences); err == nil {
			u.preferences = preferences
		} else {
			log.Printf("load update preferences: %v", err)
		}
	} else if !os.IsNotExist(err) {
		log.Printf("load update preferences: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	u.ctx = ctx
	mygo.App.OnQuit(func() {
		cancel()
		if u.timer != nil {
			u.timer.Stop()
		}
	})
	u.automatic = (u.preferences.AutomaticChecks == nil || *u.preferences.AutomaticChecks) && mygo.Updater.Enabled()
	a.scheduleUpdateCheck(10 * time.Second)
}

func (a *island) scheduleUpdateCheck(delay time.Duration) {
	u := &a.updates
	if u.timer != nil {
		u.timer.Stop()
		u.timer = nil
	}
	if u.automatic && u.phase != updateReady && u.ctx.Err() == nil {
		u.timer = time.AfterFunc(delay, func() { mygo.RunOnMain(func() { a.checkForUpdates(false) }) })
	}
}

func (a *island) saveUpdatePreferences() {
	if err := saveJSON(filepath.Join(filepath.Dir(a.statePath), "updater.json"), a.updates.preferences); err != nil {
		log.Printf("save update preferences: %v", err)
	}
}

func (a *island) setUpdatePhase(phase updatePhase) {
	a.updates.phase = phase
	if a.settingsWindow != nil {
		a.settingsWindow.Invalidate()
	}
}

func (a *island) finishUpdate(phase updatePhase, nextCheck time.Duration) {
	a.setUpdatePhase(phase)
	a.scheduleUpdateCheck(nextCheck)
}

func (a *island) checkForUpdates(manual bool) {
	u := &a.updates
	if u.phase == updateChecking && manual && !u.interactive {
		u.interactive = true
		a.setUpdatePhase(updateChecking)
		return
	}
	if (u.phase != updateIdle && u.phase != updateLatest) || u.ctx.Err() != nil {
		return
	}
	u.interactive = manual
	a.setUpdatePhase(updateChecking)
	go func() {
		update, err := mygo.Updater.Check(u.ctx)
		mygo.RunOnMain(func() {
			if u.ctx.Err() != nil {
				return
			}
			if err != nil {
				a.updateFailed("检查更新失败", err)
				return
			}
			u.preferences.LastCheck = time.Now()
			a.saveUpdatePreferences()
			switch {
			case update == nil:
				if u.interactive {
					u.announcement = "已是最新版本"
					a.finishUpdate(updateLatest, updateInterval)
				} else {
					a.finishUpdate(updateIdle, updateInterval)
				}
			case !u.interactive && update.Version == u.preferences.SkippedVersion:
				a.finishUpdate(updateIdle, updateInterval)
			case u.interactive || u.preferences.AutomaticDownloads:
				a.downloadUpdate(update)
			default:
				a.offerUpdate(update)
			}
		})
	}()
}

func (a *island) offerUpdate(update *mygo.Update) {
	a.setUpdatePhase(updateOffering)
	parent := a.settingsWindow
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: parent, Title: "软件更新", Type: mygo.MessageInfo,
			Message: fmt.Sprintf("发现新版本 %s", update.Version), Detail: update.Notes,
			Buttons: []string{"下载更新", "稍后", "跳过此版本"}, CancelButton: 1,
			CheckboxLabel: "以后自动下载更新",
		})
		mygo.RunOnMain(func() {
			u := &a.updates
			if u.ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Printf("offer update: %v", err)
				a.finishUpdate(updateIdle, time.Hour)
				return
			}
			u.preferences.AutomaticDownloads = result.CheckboxChecked
			if result.Button == 2 {
				u.preferences.SkippedVersion = update.Version
			}
			a.saveUpdatePreferences()
			if result.Button == 0 {
				u.interactive = true
				a.downloadUpdate(update)
				a.openSettings()
			} else {
				a.finishUpdate(updateIdle, updateInterval)
			}
		})
	}()
}

func (a *island) downloadUpdate(update *mygo.Update) {
	u := &a.updates
	u.percent = 0
	a.setUpdatePhase(updateDownloading)
	go func() {
		last := 0
		err := update.Install(u.ctx, func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			percent := min(100, int(100*downloaded/total))
			if percent != last {
				last = percent
				mygo.RunOnMain(func() {
					u.percent = percent
					a.setUpdatePhase(updateDownloading)
				})
			}
		})
		mygo.RunOnMain(func() {
			if u.ctx.Err() != nil {
				return
			}
			if err != nil {
				a.updateFailed("安装更新失败", err)
				return
			}
			u.announcement = "更新已安装，重启应用后生效"
			a.finishUpdate(updateReady, updateInterval)
		})
	}()
}

func (a *island) updateFailed(message string, err error) {
	log.Printf("update: %v", err)
	a.finishUpdate(updateIdle, time.Hour)
	if !a.updates.interactive {
		return
	}
	detail := "请稍后重试。"
	if errors.Is(err, mygo.ErrUpdatesDisabled) {
		detail = "请使用正式安装版，并确认应用所在位置可写。"
	}
	parent := a.settingsWindow
	go func() {
		_, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: parent, Title: "软件更新", Type: mygo.MessageError,
			Message: message, Detail: detail, Buttons: []string{"确定"},
		})
		if err != nil {
			log.Printf("show update error: %v", err)
		}
	}()
}

func (a *island) updateControl(c *ui.Context) {
	u := &a.updates
	if u.announcement != "" {
		c.Announce(u.announcement)
		u.announcement = ""
	}
	switch {
	case u.phase == updateChecking && u.interactive, u.phase == updateDownloading:
		checking := u.phase == updateChecking
		label, value := "更新下载进度", float64(u.percent)
		fraction := value / 100
		if checking {
			label, value, fraction = "正在检查更新", -1, 0.25
		} else if u.percent == 100 {
			label = "正在安装更新"
		}
		rotating := checking && !c.Preferences().ReduceMotion
		ui.Box(c.Key("update-progress")).Size(80, 40).Shrink(0).Role(ui.RoleProgress).
			Label(label).Range(0, 100, value).Draw(func(p *ui.Painter, r ui.Rect) {
			const radius, stroke = 8.5, 2.5
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			var ring ui.Path
			p.StrokePath(ring.Circle(cx, cy, radius), stroke, theme.Border)
			if fraction == 0 {
				return
			}
			start := 0.0
			if rotating {
				const turnMillis = 900
				start = 2 * math.Pi * float64(p.Now().UnixMilli()%turnMillis) / turnMillis
				p.AnimationFrameRate(30)
			}
			var arc ui.Path
			arc.MoveTo(cx+radius*float32(math.Sin(start)), cy-radius*float32(math.Cos(start)))
			steps := int(math.Ceil(48 * fraction))
			for i := 1; i <= steps; i++ {
				angle := start + 2*math.Pi*fraction*float64(i)/float64(steps)
				arc.LineTo(cx+radius*float32(math.Sin(angle)), cy-radius*float32(math.Cos(angle)))
			}
			p.StrokePath(&arc, stroke, accent)
		})
	case u.phase == updateOffering:
		ui.Text(c, "有新版本").FontSize(12).TextColor(ink)
	case u.phase == updateLatest:
		ui.Text(c, "已是最新版本").FontSize(12).TextColor(ink)
	case u.phase == updateReady:
		if ui.Button(c.Key("restart-for-update"), "重启应用").Height(40).FontSize(12).Clicked() {
			mygo.App.Relaunch()
		}
	default:
		if ui.Button(c.Key("check-for-updates"), "检查更新").Height(40).FontSize(12).Clicked() {
			a.checkForUpdates(true)
		}
	}
}
