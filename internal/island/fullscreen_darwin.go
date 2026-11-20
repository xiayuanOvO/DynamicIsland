package island

import (
	"math"
	"os"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type cgBounds struct{ X, Y, Width, Height float64 }

var (
	copyOnscreenWindows              func(uint32, uint32) objc.ID
	windowDictionaryRect             func(objc.ID, *cgBounds) bool
	releaseCF                        func(objc.ID)
	layerKey, ownerPIDKey, boundsKey objc.ID

	sharedWorkspace      = objc.RegisterName("sharedWorkspace")
	frontmostApplication = objc.RegisterName("frontmostApplication")
	processIdentifier    = objc.RegisterName("processIdentifier")
	objectForKey         = objc.RegisterName("objectForKey:")
	objectAtIndex        = objc.RegisterName("objectAtIndex:")
	arrayCount           = objc.RegisterName("count")
	integerValue         = objc.RegisterName("integerValue")
)

func init() {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return
	}
	var stringCF func(uintptr, string, uint32) objc.ID
	purego.RegisterLibFunc(&stringCF, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&releaseCF, cf, "CFRelease")
	const utf8 = 0x08000100
	layerKey = stringCF(0, "kCGWindowLayer", utf8)
	ownerPIDKey = stringCF(0, "kCGWindowOwnerPID", utf8)
	boundsKey = stringCF(0, "kCGWindowBounds", utf8)
	purego.RegisterLibFunc(&windowDictionaryRect, cg, "CGRectMakeWithDictionaryRepresentation")
	purego.RegisterLibFunc(&copyOnscreenWindows, cg, "CGWindowListCopyWindowInfo")
}

func foregroundApplicationPID() int32 {
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(sharedWorkspace)
	front := workspace.Send(frontmostApplication)
	return objc.Send[int32](front, processIdentifier)
}

func foregroundFullscreen() bool {
	if copyOnscreenWindows == nil {
		return false
	}
	pid := int(foregroundApplicationPID())
	if pid <= 0 || pid == os.Getpid() {
		return false
	}

	const onscreenWithoutDesktop = 1 | 16
	windows := copyOnscreenWindows(onscreenWithoutDesktop, 0)
	if windows == 0 {
		return false
	}
	defer releaseCF(windows)
	count := objc.Send[uint](windows, arrayCount)
	displays := mygo.Screen.Displays()
	for i := range count {
		item := windows.Send(objectAtIndex, i)
		if objc.Send[int](item.Send(objectForKey, ownerPIDKey), integerValue) != pid ||
			objc.Send[int](item.Send(objectForKey, layerKey), integerValue) != 0 {
			continue
		}
		var rect cgBounds
		if !windowDictionaryRect(item.Send(objectForKey, boundsKey), &rect) {
			continue
		}
		bounds := mygo.Rectangle{
			X: int(math.Round(rect.X)), Y: int(math.Round(rect.Y)),
			Width: int(math.Round(rect.Width)), Height: int(math.Round(rect.Height)),
		}
		for _, display := range displays {
			if windowCoversDisplay(bounds, display.Bounds) {
				return true
			}
		}
	}
	return false
}
