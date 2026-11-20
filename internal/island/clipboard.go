package island

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

const (
	clipboardHistoryLimit = 50
	clipboardTextLimit    = 32 * 1024
	clipboardImageLimit   = 16 << 20
	clipboardImageBudget  = 64 << 20
	clipboardHotkey       = "Alt+Shift+V"
	clipboardHistoryFlag  = transfer.Format("CanIncludeInClipboardHistory")
)

type clipboardEntry struct {
	Text     string          `json:"text,omitempty"`
	Image    *clipboardImage `json:"image,omitempty"`
	CopiedAt time.Time       `json:"copiedAt"`
}

type clipboardImage struct {
	ID     string `json:"id"`
	Size   int    `json:"size"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type clipboardEntryKey struct{ text, imageID string }

func (entry clipboardEntry) key() clipboardEntryKey {
	key := clipboardEntryKey{text: entry.Text}
	if entry.Image != nil {
		key.imageID = entry.Image.ID
	}
	return key
}

type clipboardState struct {
	Recording bool             `json:"recording"`
	Entries   []clipboardEntry `json:"entries,omitempty"`
}

type clipboardComponent struct {
	clipboardState
	open                bool
	openedByShortcut    bool
	shortcutUnavailable bool
	confirmClear        bool
	query               string
	focusSearch         bool
	selected            clipboardEntryKey
	pendingPaste        *clipboardEntry
	pasteTarget         clipboardPasteTarget
	loadFailed          bool
	problem             string
	stop                func()
}

func (a *island) clipboardPath() string {
	return filepath.Join(filepath.Dir(a.statePath), "clipboard.json")
}

func (a *island) clipboardImagePath(id string) string {
	return filepath.Join(a.clipboardImageDir(), id+".png")
}

func (a *island) clipboardImageDir() string {
	return filepath.Join(filepath.Dir(a.statePath), "clipboard-images")
}

func (a *island) loadClipboard() {
	history := &a.clipboard
	data, err := os.ReadFile(a.clipboardPath())
	if os.IsNotExist(err) {
		return
	}
	var state clipboardState
	if err == nil {
		err = json.Unmarshal(data, &state)
	}
	valid := err == nil && len(state.Entries) <= clipboardHistoryLimit
	seen := make(map[clipboardEntryKey]bool, len(state.Entries))
	imageBytes := 0
	for _, entry := range state.Entries {
		key := entry.key()
		if entry.CopiedAt.IsZero() || seen[key] ||
			((entry.Image == nil || entry.Text != "") && !validClipboardText(entry.Text)) {
			valid = false
			break
		}
		if img := entry.Image; img != nil {
			if _, err := hex.DecodeString(img.ID); err != nil || len(img.ID) != 2*sha256.Size ||
				img.Size <= 0 || img.Size > clipboardImageLimit || !validClipboardDimensions(img.Width, img.Height) {
				valid = false
				break
			}
			imageBytes += img.Size
		}
		seen[key] = true
	}
	if !valid || imageBytes > clipboardImageBudget {
		history.loadFailed = true
		history.problem = "剪贴板历史读取失败"
		return
	}
	history.clipboardState = state
}

func (a *island) saveClipboard(next clipboardState) bool {
	history := &a.clipboard
	if err := saveJSON(a.clipboardPath(), next); err != nil {
		log.Printf("save clipboard history: %v", err)
		history.problem = "剪贴板历史保存失败"
		return false
	}
	// Commit the list before removing any image files it used to reference.
	kept := make(map[string]bool)
	for _, entry := range next.Entries {
		if entry.Image != nil {
			kept[entry.Image.ID] = true
		}
	}
	for _, entry := range history.Entries {
		if entry.Image != nil && !kept[entry.Image.ID] {
			path := a.clipboardImagePath(entry.Image.ID)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				log.Printf("remove clipboard image: %v", err)
			}
			delete(a.previews, shelf.Key(path))
		}
	}
	if history.loadFailed {
		if err := os.RemoveAll(a.clipboardImageDir()); err != nil {
			log.Printf("reset clipboard images: %v", err)
		}
	}
	history.clipboardState = next
	history.loadFailed = false
	history.problem = ""
	return true
}

func (a *island) syncClipboardRecording() {
	history := &a.clipboard
	if !history.Recording {
		if history.stop != nil {
			history.stop()
			history.stop = nil
		}
		mygo.GlobalShortcut.Unregister(clipboardHotkey)
		history.shortcutUnavailable = false
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
			err := mygo.GlobalShortcut.RegisterWith(clipboardHotkey, mygo.GlobalShortcutOptions{Description: "打开剪贴板历史"}, func() {
				a.openClipboard(true)
			})
			if err != nil {
				history.shortcutUnavailable = true
				log.Printf("clipboard shortcut: %v", err)
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
	next := a.clipboard.clipboardState
	if !next.Recording {
		return false
	}
	entry := clipboardEntry{CopiedAt: copiedAt}
	if text, err := data.Read(transfer.Text); err == nil && validClipboardText(string(text)) {
		entry.Text = string(text)
	}
	bitmap, _ := data.Read(transfer.PNG)
	if len(bitmap) > 0 && len(bitmap) <= clipboardImageLimit {
		if config, err := png.DecodeConfig(bytes.NewReader(bitmap)); err == nil && validClipboardDimensions(config.Width, config.Height) {
			id := sha256.Sum256(bitmap)
			entry.Image = &clipboardImage{ID: hex.EncodeToString(id[:]), Size: len(bitmap), Width: config.Width, Height: config.Height}
		}
	}
	key := entry.key()
	if (entry.Text == "" && entry.Image == nil) || (len(next.Entries) > 0 && next.Entries[0].key() == key) {
		return false
	}
	imageBytes := 0
	if entry.Image != nil {
		imageBytes = entry.Image.Size
		if err := saveStateFile(a.clipboardImagePath(entry.Image.ID), bitmap); err != nil {
			log.Printf("save clipboard image: %v", err)
			a.clipboard.problem = "剪贴板图片保存失败"
			return false
		}
	}
	entries := make([]clipboardEntry, 1, min(len(next.Entries)+1, clipboardHistoryLimit))
	entries[0] = entry
	for _, previous := range next.Entries {
		if len(entries) == clipboardHistoryLimit {
			break
		}
		if previous.key() == key {
			continue
		}
		if previous.Image != nil {
			if imageBytes+previous.Image.Size > clipboardImageBudget {
				continue
			}
			imageBytes += previous.Image.Size
		}
		entries = append(entries, previous)
	}
	next.Entries = entries
	if a.saveClipboard(next) {
		return true
	}
	if entry.Image != nil && !slices.ContainsFunc(a.clipboard.Entries, func(previous clipboardEntry) bool {
		return previous.Image != nil && previous.Image.ID == entry.Image.ID
	}) {
		if err := os.Remove(a.clipboardImagePath(entry.Image.ID)); err != nil && !os.IsNotExist(err) {
			log.Printf("discard unsaved clipboard image: %v", err)
		}
	}
	return false
}

func (a *island) clipboardData(entry clipboardEntry) (transfer.Data, error) {
	if entry.Image == nil {
		return transfer.TextData(entry.Text), nil
	}
	file, err := os.Open(a.clipboardImagePath(entry.Image.ID))
	if err != nil {
		return transfer.Data{}, err
	}
	defer file.Close()
	bitmap, err := io.ReadAll(io.LimitReader(file, int64(entry.Image.Size)+1))
	if err != nil {
		return transfer.Data{}, err
	}
	id := sha256.Sum256(bitmap)
	if len(bitmap) != entry.Image.Size || hex.EncodeToString(id[:]) != entry.Image.ID {
		return transfer.Data{}, errors.New("saved clipboard image has changed")
	}
	representations := []transfer.Representation{transfer.Bytes(transfer.PNG, bitmap)}
	if entry.Text != "" {
		representations = append(representations, transfer.Bytes(transfer.Text, []byte(entry.Text)))
	}
	return transfer.New(transfer.NewItem(representations...)), nil
}

func validClipboardDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= 24_000_000/height
}

func validClipboardText(text string) bool {
	return len(text) <= clipboardTextLimit && utf8.ValidString(text) && strings.TrimSpace(text) != "" && !strings.ContainsRune(text, '\x00')
}
