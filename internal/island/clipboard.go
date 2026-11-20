package island

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

const (
	clipboardTextLimit     = 32 * 1024
	clipboardImageLimit    = 16 << 20
	defaultClipboardHotkey = "Alt+Shift+V"
	clipboardHistoryFlag   = transfer.Format("CanIncludeInClipboardHistory")
)

type clipboardEntry struct {
	ID            int64
	Preview       string
	Width, Height int
	CopiedAt      time.Time
}

type clipboardState struct {
	Recording bool   `json:"recording"`
	Shortcut  string `json:"shortcut,omitempty"`
}

func (s clipboardState) hotkey() string {
	if s.Shortcut != "" {
		return s.Shortcut
	}
	return defaultClipboardHotkey
}

type clipboardComponent struct {
	clipboardState
	store              *clipboardStore
	entries            []clipboardEntry
	hasEntries         bool
	list               ui.ListState
	open               bool
	openedByShortcut   bool
	registeredShortcut string
	editingShortcut    bool
	shortcutProblem    string
	confirmClear       bool
	query              string
	focusSearch        bool
	selected           int64
	hoverSelection     int64
	pendingPaste       *clipboardEntry
	pasteTarget        clipboardPasteTarget
	loadFailed         bool
	problem            string
	stop               func()
}

func (a *island) clipboardPath() string {
	return filepath.Join(filepath.Dir(a.statePath), "clipboard.db")
}

func (a *island) loadClipboard() {
	history := &a.clipboard
	store, err := openClipboardStore(a.clipboardPath())
	if err == nil {
		now := time.Now()
		history.clipboardState, err = store.settings()
		if errors.Is(err, sql.ErrNoRows) {
			dir := filepath.Dir(a.statePath)
			legacyPath := filepath.Join(dir, "clipboard.json")
			legacyImages := filepath.Join(dir, "clipboard-images")
			var migrated bool
			migrated, err = store.migrate(legacyPath, legacyImages, now)
			if err == nil {
				history.clipboardState, err = store.settings()
				if err == nil && migrated {
					if removeErr := os.Remove(legacyPath); removeErr != nil {
						log.Printf("remove migrated clipboard list: %v", removeErr)
					}
					if removeErr := os.RemoveAll(legacyImages); removeErr != nil {
						log.Printf("remove migrated clipboard images: %v", removeErr)
					}
				}
			}
		}
		if err == nil {
			_, err = store.expire(now)
		}
		if err != nil {
			store.Close()
		}
	}
	if err != nil {
		log.Printf("load clipboard history: %v", err)
		history.clipboardState = clipboardState{}
		history.loadFailed, history.problem = true, "剪贴板历史读取失败"
		return
	}
	history.store = store
	history.loadFailed, history.problem = false, ""
}

func (a *island) resetClipboard() bool {
	dir := filepath.Dir(a.statePath)
	for _, name := range []string{"clipboard.db", "clipboard.db-journal", "clipboard.json", "clipboard-images"} {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			log.Printf("reset clipboard history: %v", err)
			a.clipboard.problem = "清空历史失败"
			return false
		}
	}
	a.loadClipboard()
	return !a.clipboard.loadFailed
}

func (a *island) saveClipboardSettings(next clipboardState) bool {
	if err := a.clipboard.store.saveSettings(next); err != nil {
		log.Printf("save clipboard settings: %v", err)
		a.clipboard.problem = "剪贴板设置保存失败"
		return false
	}
	a.clipboard.clipboardState = next
	a.clipboard.problem = ""
	return true
}

func (a *island) refreshClipboardEntries(now time.Time) {
	history := &a.clipboard
	entries, err := history.store.entries(history.query, now)
	if err != nil {
		log.Printf("read clipboard entries: %v", err)
		history.problem = "剪贴板历史读取失败"
		return
	}
	if len(entries) > 0 || strings.TrimSpace(history.query) == "" {
		history.hasEntries = len(entries) != 0
	} else if err := history.store.QueryRow("SELECT EXISTS(SELECT 1 FROM entries WHERE copied_at > ?)",
		now.Add(-clipboardRetention).UnixNano()).Scan(&history.hasEntries); err != nil {
		log.Printf("check clipboard entries: %v", err)
		history.problem = "剪贴板历史读取失败"
		return
	}
	history.entries, history.problem = entries, ""
}

func (a *island) watchClipboardExpiry() {
	done := make(chan struct{})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { close(done) })
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mygo.RunOnMain(func() {
					history := &a.clipboard
					if history.store == nil {
						return
					}
					now := time.Now()
					changed, err := history.store.expire(now)
					if err != nil {
						log.Printf("expire clipboard entries: %v", err)
					}
					if changed && history.open {
						a.refreshClipboardEntries(now)
						a.win.Invalidate()
					}
				})
			}
		}
	}()
}

func (a *island) syncClipboardRecording() {
	history := &a.clipboard
	if !history.Recording {
		if history.stop != nil {
			history.stop()
			history.stop = nil
		}
		if history.registeredShortcut != "" {
			mygo.GlobalShortcut.Unregister(history.registeredShortcut)
			history.registeredShortcut = ""
		}
		return
	}
	if history.stop == nil {
		history.stop = mygo.Clipboard.OnChanged(func() {
			previousProblem := history.problem
			recorded := a.recordClipboard(readClipboard(mygo.Clipboard.Formats(), mygo.Clipboard.Read), time.Now())
			if history.open && (recorded || history.problem != previousProblem) {
				a.win.Invalidate()
			}
		})
		if a.win != nil {
			keys := history.hotkey()
			if err := a.registerClipboardShortcut(keys); err != nil {
				log.Printf("clipboard shortcut: %v", err)
			} else {
				history.registeredShortcut = keys
			}
		}
	}
}

func readClipboard(formats []transfer.Format, read func(...transfer.Format) (transfer.Data, error)) transfer.Data {
	var requested []transfer.Format
	var hasHistoryFlag bool
	for _, format := range formats {
		switch format {
		case "org.nspasteboard.ConcealedType", "org.nspasteboard.TransientType", "org.nspasteboard.AutoGeneratedType",
			"ExcludeClipboardContentFromMonitorProcessing", "x-kde-passwordManagerHint":
			return transfer.Data{}
		case transfer.Text, transfer.PNG:
			requested = append(requested, format)
		case clipboardHistoryFlag:
			hasHistoryFlag = true
		}
	}
	if len(requested) == 0 {
		return transfer.Data{}
	}
	if hasHistoryFlag {
		requested = append(requested, clipboardHistoryFlag)
	}
	data, err := read(requested...)
	if err != nil {
		return transfer.Data{}
	}
	if hasHistoryFlag {
		flag, err := data.Read(clipboardHistoryFlag)
		if err != nil || len(flag) < 4 || binary.LittleEndian.Uint32(flag) == 0 {
			return transfer.Data{}
		}
	}
	return data
}

// The watcher and the panel both update history on the main thread.
func (a *island) recordClipboard(data transfer.Data, copiedAt time.Time) bool {
	history := &a.clipboard
	if !history.Recording {
		return false
	}
	record := clipboardRecord{copiedAt: copiedAt}
	if text, err := data.Read(transfer.Text); err == nil && validClipboardText(string(text)) {
		record.text = string(text)
	}
	bitmap, _ := data.Read(transfer.PNG)
	if len(bitmap) > 0 && len(bitmap) <= clipboardImageLimit {
		if config, err := png.DecodeConfig(bytes.NewReader(bitmap)); err == nil && validClipboardDimensions(config.Width, config.Height) {
			record.png, record.width, record.height = bitmap, config.Width, config.Height
		}
	}
	if record.text == "" && len(record.png) == 0 {
		return false
	}
	changed, err := history.store.record(record)
	if err != nil {
		log.Printf("save clipboard entry: %v", err)
		history.problem = "剪贴板历史保存失败"
		return false
	}
	history.problem = ""
	if changed && history.open {
		a.refreshClipboardEntries(copiedAt)
	}
	return changed
}

func (a *island) clipboardData(entry clipboardEntry) (transfer.Data, error) {
	var text string
	var bitmap []byte
	if err := a.clipboard.store.QueryRow("SELECT text, image FROM entries WHERE id=? AND copied_at > ?",
		entry.ID, time.Now().Add(-clipboardRetention).UnixNano()).Scan(&text, &bitmap); err != nil {
		return transfer.Data{}, err
	}
	if len(bitmap) == 0 {
		return transfer.TextData(text), nil
	}
	representations := []transfer.Representation{transfer.Bytes(transfer.PNG, bitmap)}
	if text != "" {
		representations = append(representations, transfer.Bytes(transfer.Text, []byte(text)))
	}
	return transfer.New(transfer.NewItem(representations...)), nil
}

func validClipboardDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= 24_000_000/height
}

func validClipboardText(text string) bool {
	return len(text) <= clipboardTextLimit && utf8.ValidString(text) && strings.TrimSpace(text) != "" && !strings.ContainsRune(text, '\x00')
}
