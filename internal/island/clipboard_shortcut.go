package island

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *island) clipboardWindowBlurred() {
	if a.clipboard.open && a.clipboard.openedByShortcut {
		a.setExpanded(false)
	}
}

func (a *island) registerClipboardShortcut(keys string) error {
	return mygo.GlobalShortcut.RegisterWith(keys, mygo.GlobalShortcutOptions{Description: "打开剪贴板历史"}, func() {
		if a.clipboard.editingShortcut && a.settingsWindow != nil && a.settingsWindow.IsFocused() {
			a.clipboard.editingShortcut = false
			a.clipboard.shortcutProblem = ""
			a.settingsWindow.Invalidate()
			return
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
	settingsRow(c.Key("clipboard-shortcut")).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "快捷键").FontSize(13).TextColor(ink)
			problem := history.shortcutProblem
			if problem == "" && history.Recording && a.win != nil && history.registeredShortcut == "" {
				problem = "剪贴板快捷键不可用"
			}
			if problem != "" {
				ui.Text(c, problem).FontSize(11).TextColor(ui.Hex("#a65b50"))
			}
		})
		label := strings.ReplaceAll(history.hotkey(), "+", " + ")
		if runtime.GOOS == "darwin" {
			label = strings.NewReplacer("Super", "Command", "Ctrl", "Control", "Alt", "Option").Replace(label)
		} else if runtime.GOOS == "windows" {
			label = strings.ReplaceAll(label, "Super", "Win")
		}
		if history.editingShortcut {
			label = "按下快捷键…"
		}
		button := ui.Button(c.Key("edit-shortcut"), label).Height(36).FontSize(12).
			Label("修改剪贴板快捷键 " + label)
		if button.FocusLeft() {
			history.editingShortcut, history.shortcutProblem = false, ""
		}
		if button.Clicked() {
			history.editingShortcut, history.shortcutProblem = true, ""
			button.Focus()
		}
		button.HandleInput(func(event ui.InputEvent) bool {
			if !history.editingShortcut || event.Kind != ui.InputKeyDown {
				return false
			}
			if event.Key == ui.KeyTab && event.Mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 {
				return false
			}
			if event.Key == ui.KeyEscape && event.Mods == 0 {
				history.editingShortcut, history.shortcutProblem = false, ""
				return true
			}
			if event.Repeat || (event.Key == ui.KeyUnknown && event.PhysicalKey == ui.KeyUnknown) {
				return true
			}
			keys := clipboardShortcutFromKey(event)
			if keys == "" {
				history.shortcutProblem = "请使用组合键或功能键"
			} else if a.changeClipboardShortcut(keys) {
				history.editingShortcut, history.shortcutProblem = false, ""
			}
			return true
		})
	})
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
