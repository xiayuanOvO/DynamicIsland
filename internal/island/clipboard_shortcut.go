package island

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *island) clipboardShortcuts(c *ui.Context) {
	history := &a.clipboard
	if c.Shortcut(0, ui.KeyEscape) {
		if history.confirmClear {
			history.confirmClear = false
			history.focusSearch = true
		} else {
			history.open = false
		}
		return
	}
	if history.confirmClear || len(history.entries) == 0 {
		return
	}

	// The window can receive keys before the animated search field takes focus.
	up := c.Shortcut(0, ui.KeyUp)
	down := c.Shortcut(0, ui.KeyDown)
	if !up && !down {
		return
	}
	selected := history.selectedIndex()
	if up {
		selected = max(0, selected-1)
	} else {
		if selected+1 >= len(history.entries)-1 {
			a.loadMoreClipboardEntries()
		}
		selected = min(len(history.entries)-1, selected+1)
	}
	history.selected = history.entries[selected].ID
	history.focusSearch = true
}

func (a *island) clipboardWindowBlurred() {
	if a.clipboard.open && a.clipboard.openedByShortcut {
		a.setExpanded(false)
	}
}

func (a *island) registerToggleShortcut(keys string) error {
	err := mygo.GlobalShortcut.RegisterWith(keys, mygo.GlobalShortcutOptions{Description: "展开 / 收起灵动岛"}, func() {
		if a.settingsWindow != nil && a.settingsWindow.IsFocused() {
			if a.editingHotkey {
				a.editingHotkey, a.hotkeyProblem = false, ""
				a.settingsWindow.Invalidate()
				return
			}
			if a.clipboard.editingShortcut {
				a.clipboard.shortcutProblem = "快捷键已被灵动岛使用"
				a.settingsWindow.Invalidate()
				return
			}
		}
		a.toggle()
	})
	if err != nil {
		log.Printf("global shortcut: %v", err)
	}
	return err
}

func (a *island) changeToggleShortcut(keys string) bool {
	previous := a.settings.toggleHotkey()
	bind := a.win != nil
	if keys == previous && (!bind || keys == a.registeredHotkey) {
		a.hotkeyFailed = false
		return true
	}
	if bind {
		if err := a.registerToggleShortcut(keys); err != nil {
			a.hotkeyProblem = "无法注册此快捷键"
			return false
		}
	}
	if keys != previous {
		next := a.settings
		next.ToggleShortcut = keys
		if err := next.save(a.settingsPath()); err != nil {
			if bind {
				mygo.GlobalShortcut.Unregister(keys)
			}
			log.Printf("save toggle shortcut: %v", err)
			a.hotkeyProblem = "快捷键保存失败"
			return false
		}
		a.settings = next
		a.settingsSaveFailed = false
	}
	if bind {
		if a.registeredHotkey != "" {
			mygo.GlobalShortcut.Unregister(a.registeredHotkey)
		}
		a.registeredHotkey = keys
	}
	a.hotkeyFailed = false
	return true
}

func (a *island) toggleShortcutSettings(c *ui.Context) {
	fallback := ""
	if a.hotkeyFailed {
		fallback = "快捷键被占用"
	}
	shortcutSettingsRow(c, "hotkey-settings", "展开 / 收起快捷键", "edit-toggle-shortcut", "修改展开 / 收起快捷键",
		a.settings.toggleHotkey(), fallback, &a.editingHotkey, &a.hotkeyProblem, a.changeToggleShortcut)
}

func (a *island) registerClipboardShortcut(keys string) error {
	return mygo.GlobalShortcut.RegisterWith(keys, mygo.GlobalShortcutOptions{Description: "打开剪贴板历史"}, func() {
		if a.settingsWindow != nil && a.settingsWindow.IsFocused() {
			if a.clipboard.editingShortcut {
				a.clipboard.editingShortcut, a.clipboard.shortcutProblem = false, ""
				a.settingsWindow.Invalidate()
				return
			}
			if a.editingHotkey {
				a.hotkeyProblem = "快捷键已被灵动岛使用"
				a.settingsWindow.Invalidate()
				return
			}
		}
		a.openClipboard(true)
	})
}

func (a *island) changeClipboardShortcut(keys string) bool {
	history := &a.clipboard
	if keys == "Super+V" && runtime.GOOS == "darwin" || keys == "Ctrl+V" && runtime.GOOS != "darwin" {
		// Automatic paste sends this combination to the original window.
		history.shortcutProblem = "此组合用于系统粘贴"
		return false
	}
	previous := history.hotkey()
	bind := history.Recording && a.win != nil
	if keys == previous && (!bind || keys == history.registeredShortcut) {
		return true
	}
	if bind {
		if err := a.registerClipboardShortcut(keys); err != nil {
			log.Printf("register clipboard shortcut: %v", err)
			history.shortcutProblem = "无法注册此快捷键"
			return false
		}
	}
	if keys != previous {
		next := history.clipboardState
		next.Shortcut = keys
		if err := history.store.saveSettings(next); err != nil {
			if bind {
				mygo.GlobalShortcut.Unregister(keys)
			}
			log.Printf("save clipboard shortcut: %v", err)
			history.shortcutProblem = "快捷键保存失败"
			return false
		}
		history.clipboardState = next
	}
	if bind {
		if history.registeredShortcut != "" {
			mygo.GlobalShortcut.Unregister(history.registeredShortcut)
		}
		history.registeredShortcut = keys
	}
	return true
}

func (a *island) clipboardShortcutSettings(c *ui.Context) {
	history := &a.clipboard
	fallback := ""
	if history.Recording && a.win != nil && history.registeredShortcut == "" {
		fallback = "剪贴板快捷键不可用"
	}
	shortcutSettingsRow(c, "clipboard-shortcut", "快捷键", "edit-shortcut", "修改剪贴板快捷键",
		history.hotkey(), fallback, &history.editingShortcut, &history.shortcutProblem, a.changeClipboardShortcut)
}

func shortcutSettingsRow(c *ui.Context, rowKey, title, buttonKey, ariaPrefix, currentKeys, fallbackProblem string, editing *bool, problem *string, apply func(string) bool) {
	settingsRow(c.Key(rowKey)).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13).TextColor(c.Theme().Text)
			msg := *problem
			if msg == "" {
				msg = fallbackProblem
			}
			if msg != "" {
				ui.Text(c, msg).FontSize(11).TextColor(colorsFor(c.Theme().Dark).problem)
			}
		})
		label := formatHotkey(currentKeys)
		if *editing {
			label = "按下快捷键…"
		}
		button := ui.Button(c.Key(buttonKey), label).Height(36).FontSize(12).
			Label(ariaPrefix + " " + label)
		if button.FocusLeft() {
			*editing, *problem = false, ""
		}
		if button.Clicked() {
			*editing, *problem = true, ""
			button.Focus()
		}
		button.HandleInput(func(event ui.InputEvent) bool {
			if !*editing || event.Kind != ui.InputKeyDown {
				return false
			}
			if event.Key == ui.KeyTab && event.Mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 {
				return false
			}
			if event.Key == ui.KeyEscape && event.Mods == 0 {
				*editing, *problem = false, ""
				return true
			}
			if event.Repeat || (event.Key == ui.KeyUnknown && event.PhysicalKey == ui.KeyUnknown) {
				return true
			}
			keys := clipboardShortcutFromKey(event)
			if keys == "" {
				*problem = "请使用组合键或功能键"
			} else if apply(keys) {
				*editing, *problem = false, ""
			}
			return true
		})
	})
}

func formatHotkey(keys string) string {
	label := strings.ReplaceAll(keys, "+", " + ")
	if runtime.GOOS == "darwin" {
		return strings.NewReplacer("CmdOrCtrl", "Command", "Super", "Command", "Ctrl", "Control", "Alt", "Option").Replace(label)
	}
	if runtime.GOOS == "windows" {
		return strings.NewReplacer("CmdOrCtrl", "Ctrl", "Super", "Win").Replace(label)
	}
	return strings.ReplaceAll(label, "CmdOrCtrl", "Ctrl")
}

func clipboardShortcutFromKey(event ui.InputEvent) string {
	key := event.Key
	if event.PhysicalKey != ui.KeyUnknown && (runtime.GOOS == "darwin" || key == ui.KeyUnknown) {
		key = event.PhysicalKey
	}
	functionKey := key >= ui.KeyF1 && key <= ui.KeyF12
	if !functionKey && event.Mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 {
		return ""
	}
	var name string
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		name = string(rune('0' + key - ui.Key0))
	case functionKey:
		name = fmt.Sprintf("F%d", key-ui.KeyF1+1)
	default:
		name = map[ui.Key]string{
			ui.KeySpace: "Space", ui.KeyTab: "Tab", ui.KeyEnter: "Enter", ui.KeyEscape: "Escape",
			ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete", ui.KeyInsert: "Insert",
			ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
			ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
			ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/",
			ui.KeySemicolon: ";", ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
		}[key]
	}
	if name == "" {
		return ""
	}
	var parts []string
	for _, modifier := range []struct {
		bit  ui.Modifiers
		name string
	}{{ui.Super, "Super"}, {ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}} {
		if event.Mods&modifier.bit != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}
