package island

import (
	"os"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	getGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	setForegroundWindow      = user32.NewProc("SetForegroundWindow")
	sendKeyboardInput        = user32.NewProc("SendInput")
)

type clipboardPasteTarget struct {
	window, focus uintptr
	pid           uint32
}

func captureClipboardPasteTarget() clipboardPasteTarget {
	window, _, _ := getForegroundWindow.Call()
	if window == 0 {
		return clipboardPasteTarget{}
	}
	var pid uint32
	thread, _, _ := getWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&pid)))
	if thread == 0 || pid == uint32(os.Getpid()) {
		return clipboardPasteTarget{}
	}
	var info struct {
		size, flags                uint32
		active, focus, capture     uintptr
		menuOwner, moveSize, caret uintptr
		caretRect                  [4]int32
	}
	info.size = uint32(unsafe.Sizeof(info))
	getGUIThreadInfo.Call(thread, uintptr(unsafe.Pointer(&info)))
	return clipboardPasteTarget{window: window, focus: info.focus, pid: pid}
}

func (target clipboardPasteTarget) restore(win *mygo.Window) bool {
	current, _, _ := getForegroundWindow.Call()
	if target.window == 0 {
		if current == win.NativeHandle() {
			win.Hide()
		}
		return false
	}
	if current == target.window {
		return true
	}
	if current != win.NativeHandle() {
		return false
	}
	var pid uint32
	getWindowThreadProcessID.Call(target.window, uintptr(unsafe.Pointer(&pid)))
	if pid != target.pid {
		return false
	}
	ok, _, _ := setForegroundWindow.Call(target.window)
	if ok == 0 {
		win.Hide()
	}
	return true // paste rechecks the foreground window and its focused control.
}

func (target clipboardPasteTarget) paste() {
	current := captureClipboardPasteTarget()
	if current.window != target.window || current.pid != target.pid || (target.focus != 0 && current.focus != target.focus) {
		return
	}
	const (
		vkShift = 0x10
		vkCtrl  = 0x11
		vkAlt   = 0x12
		vkLWin  = 0x5B
		vkRWin  = 0x5C
		vkV     = 0x56
		keyUp   = 0x0002
	)
	for _, key := range []uintptr{vkShift, vkCtrl, vkAlt, vkLWin, vkRWin} {
		if state, _, _ := getAsyncKeyState.Call(key); int16(state) < 0 {
			return
		}
	}
	type keyboardInput struct {
		key, scan   uint16
		flags, time uint32
		extra       uintptr
	}
	type input struct {
		kind     uint32
		keyboard keyboardInput
		// The Win32 INPUT union is sized to MOUSEINPUT, which is 8 bytes larger.
		padding [8]byte
	}
	events := [...]input{
		{kind: 1, keyboard: keyboardInput{key: vkCtrl}},
		{kind: 1, keyboard: keyboardInput{key: vkV}},
		{kind: 1, keyboard: keyboardInput{key: vkV, flags: keyUp}},
		{kind: 1, keyboard: keyboardInput{key: vkCtrl, flags: keyUp}},
	}
	sendKeyboardInput.Call(uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), unsafe.Sizeof(events[0]))
}
