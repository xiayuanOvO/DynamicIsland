package island

import (
	"os"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type clipboardPasteTarget struct{ pid int32 }

var (
	createPasteKeyEvent func(objc.ID, uint16, bool) objc.ID
	setPasteEventFlags  func(objc.ID, uint64)
	postPasteEvent      func(int32, objc.ID)
	canPostPasteEvent   func() bool
	requestPasteAccess  func() bool
	applicationForPID   = objc.RegisterName("runningApplicationWithProcessIdentifier:")
	activateApplication = objc.RegisterName("activateWithOptions:")
)

func init() {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return
	}
	purego.RegisterLibFunc(&createPasteKeyEvent, cg, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&setPasteEventFlags, cg, "CGEventSetFlags")
	purego.RegisterLibFunc(&postPasteEvent, cg, "CGEventPostToPid")
	purego.RegisterLibFunc(&canPostPasteEvent, cg, "CGPreflightPostEventAccess")
	purego.RegisterLibFunc(&requestPasteAccess, cg, "CGRequestPostEventAccess")
}

func captureClipboardPasteTarget() clipboardPasteTarget {
	pid := foregroundApplicationPID()
	if pid <= 0 || int(pid) == os.Getpid() {
		return clipboardPasteTarget{}
	}
	return clipboardPasteTarget{pid: pid}
}

func (target clipboardPasteTarget) restore(win *mygo.Window) bool {
	front := foregroundApplicationPID()
	if win.IsFocused() {
		// Ordering a nonactivating panel to the back leaves it key.
		win.Hide()
	}
	if target.pid == 0 || (front != target.pid && int(front) != os.Getpid()) || canPostPasteEvent == nil {
		return false
	}
	if !canPostPasteEvent() {
		requestPasteAccess()
		return false
	}
	if front != target.pid && foregroundApplicationPID() != target.pid {
		app := objc.Send[objc.ID](objc.ID(objc.GetClass("NSRunningApplication")), applicationForPID, target.pid)
		if app != 0 {
			objc.Send[bool](app, activateApplication, uintptr(0))
		}
	}
	return true
}

func (target clipboardPasteTarget) paste() {
	if foregroundApplicationPID() != target.pid || releaseCF == nil {
		return
	}
	const keyV, commandFlag = 9, 1 << 20
	down := createPasteKeyEvent(0, keyV, true)
	up := createPasteKeyEvent(0, keyV, false)
	if down != 0 {
		defer releaseCF(down)
	}
	if up != 0 {
		defer releaseCF(up)
	}
	if down == 0 || up == 0 {
		return
	}
	setPasteEventFlags(down, commandFlag)
	setPasteEventFlags(up, commandFlag)
	postPasteEvent(target.pid, down)
	postPasteEvent(target.pid, up)
}
