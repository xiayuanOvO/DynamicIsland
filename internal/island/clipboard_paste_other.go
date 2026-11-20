//go:build !darwin && !windows

package island

import "github.com/egoist/mygo"

type clipboardPasteTarget struct{}

func captureClipboardPasteTarget() clipboardPasteTarget { return clipboardPasteTarget{} }
func (clipboardPasteTarget) restore(win *mygo.Window) bool {
	if win.IsFocused() {
		win.Hide()
	}
	return false
}

func (clipboardPasteTarget) paste() {}
