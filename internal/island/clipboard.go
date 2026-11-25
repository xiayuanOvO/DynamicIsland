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
	"slices"
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
	clipboardPageSize      = 50
	clipboardPreviewRows   = 3
	defaultClipboardHotkey = "Alt+Shift+V"
	clipboardHistoryFlag   = transfer.Format("CanIncludeInClipboardHistory")
)

type clipboardEntry struct {
	ID            int64
	Preview       string
	Width, Height int
	CopiedAt      time.Time
	Pinned        bool
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
	preview            []clipboardEntry
	previewLoaded      bool
	hasEntries         bool
	hasMore            bool
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

func (h *clipboardComponent) selectedIndex() int {
	return max(0, slices.IndexFunc(h.entries, func(entry clipboardEntry) bool { return entry.ID == h.selected }))
}

func (h *clipboardComponent) previewHeight() int {
	if !h.Recording {
		return 0
	}
	rows := min(len(h.preview), clipboardPreviewRows)
	return max(todoPreviewMinHeight, todoPreviewHeader+rows*todoPreviewRowHeight+2*todoPreviewPadding)
}

func (a *island) refreshClipboardPreview() {
	history := &a.clipboard
	if history.store == nil {
		history.preview = nil
		history.previewLoaded = false
		return
	}
	entries, _, err := history.store.entryPage("", clipboardEntry{}, clipboardLargePreviewRows)
	if err != nil {
		log.Printf("read clipboard preview: %v", err)
		return
	}
	history.preview = entries
	history.previewLoaded = true
}

func (a *island) clipboardPath() string {
	return filepath.Join(filepath.Dir(a.statePath), "clipboard.db")
}

func (a *island) loadClipboard() {
	history := &a.clipboard
	store, err := openClipboardStore(a.clipboardPath())
	if err == nil {
		history.clipboardState, err = store.settings()
		if errors.Is(err, sql.ErrNoRows) {
			dir := filepath.Dir(a.statePath)
			legacyPath := filepath.Join(dir, "clipboard.json")
			legacyImages := filepath.Join(dir, "clipboard-images")
			var migrated bool
			migrated, err = store.migrate(legacyPath, legacyImages)
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
	a.refreshClipboardPreview()
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
	if next.Recording {
		a.refreshClipboardPreview()
	}
	return true
}

func (a *island) refreshClipboardEntries(reset bool) {
	history := &a.clipboard
	if reset {
		history.entries, history.hasMore = nil, false
		history.selected, history.hoverSelection = 0, 0
		history.list = ui.ListState{}
	}
	entries, more, err := history.store.entryPage(history.query, clipboardEntry{}, max(clipboardPageSize, len(history.entries)))
	if err != nil {
		log.Printf("read clipboard entries: %v", err)
		history.problem = "剪贴板历史读取失败"
		return
	}
	if len(entries) > 0 || strings.TrimSpace(history.query) == "" {
		history.hasEntries = len(entries) != 0
	} else if err := history.store.QueryRow("SELECT EXISTS(SELECT 1 FROM entries)").Scan(&history.hasEntries); err != nil {
		log.Printf("check clipboard entries: %v", err)
		history.problem = "剪贴板历史读取失败"
		return
	}
	history.entries, history.hasMore, history.problem = entries, more, ""
	a.refreshClipboardPreview()
}

func (a *island) loadMoreClipboardEntries() {
	history := &a.clipboard
	if !history.hasMore {
		return
	}
	after := history.entries[len(history.entries)-1]
	entries, more, err := history.store.entryPage(history.query, after, clipboardPageSize)
	if err != nil {
		log.Printf("read older clipboard entries: %v", err)
		history.problem = "剪贴板历史读取失败"
		return
	}
	history.entries = append(history.entries, entries...)
	history.hasMore, history.problem = more, ""
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
				if a.settingsWindow != nil {
					a.settingsWindow.Invalidate()
				}
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
	if changed {
		if history.open {
			a.refreshClipboardEntries(false)
		} else {
			a.refreshClipboardPreview()
		}
	}
	return changed
}

func (a *island) clipboardData(entry clipboardEntry) (transfer.Data, error) {
	var text string
	var bitmap []byte
	if err := a.clipboard.store.QueryRow("SELECT text, image FROM entries WHERE id=?", entry.ID).Scan(&text, &bitmap); err != nil {
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
