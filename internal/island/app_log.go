package island

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

const (
	appLogMaxLines = 300
	appLogMaxBytes = 128 << 10
)

type appLogStore struct {
	mu    sync.Mutex
	path  string
	lines []string
}

var appLogs = &appLogStore{}

func initAppLogging(statePath string) {
	appLogs.init(filepath.Join(filepath.Dir(statePath), "app.log"))
	log.SetOutput(io.MultiWriter(os.Stderr, appLogs))
}

func (l *appLogStore) init(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.path = path
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	raw := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(raw) > appLogMaxLines {
		raw = raw[len(raw)-appLogMaxLines:]
	}
	l.lines = raw
}

func (l *appLogStore) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	text := string(bytes.TrimRight(p, "\r\n"))
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			l.lines = append(l.lines, line)
		}
		if len(l.lines) > appLogMaxLines {
			l.lines = l.lines[len(l.lines)-appLogMaxLines:]
		}
	}
	if l.path != "" {
		if err := os.MkdirAll(filepath.Dir(l.path), 0700); err == nil {
			f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err == nil {
				_, _ = f.Write(p)
				info, statErr := f.Stat()
				f.Close()
				if statErr == nil && info.Size() > appLogMaxBytes {
					_ = os.WriteFile(l.path, []byte(strings.Join(l.lines, "\n")+"\n"), 0600)
				}
			}
		}
	}
	return len(p), nil
}

func (l *appLogStore) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func (l *appLogStore) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.lines)
}

func (l *appLogStore) clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = nil
	if l.path != "" {
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (a *island) appLogSettings(c *ui.Context) {
	count := appLogs.count()
	settingsRow(c.Key("app-log-settings")).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "运行日志").FontSize(13).TextColor(c.Theme().Text)
			if a.logClearErr != "" {
				ui.Text(c, a.logClearErr).FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem)
			}
		})
		toggleLabel := "查看日志"
		if a.showLogs {
			toggleLabel = "收起日志"
		}
		if ui.Button(c.Key("toggle-logs"), toggleLabel).Height(34).FontSize(12).Clicked() {
			a.showLogs = !a.showLogs
			a.logClearErr = ""
		}
		if ui.Button(c.Key("clear-logs"), "清理日志").Height(34).FontSize(12).Disabled(count == 0).Clicked() {
			if err := appLogs.clear(); err != nil {
				a.logClearErr = "清理日志失败"
			} else {
				a.logClearErr = ""
				c.Announce("日志已清理")
			}
		}
	})
	if a.showLogs {
		settingsDivider(c)
		content := appLogs.text()
		if content == "" {
			content = "暂无日志"
		}
		ui.Scroll(c.Key("app-log-scroll")).FillWidth().Height(160).Padding(10, 14).Children(func() {
			ui.Text(c, content).FillWidth().FontSize(11).TextColor(c.Theme().TextMuted)
		})
	}
}
