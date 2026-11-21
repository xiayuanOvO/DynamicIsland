//go:build darwin

package island

import (
	"log"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

func startWindowDisplayLink(a *island) func() {
	window := objc.ID(a.win.NativeHandle())
	create := objc.RegisterName("displayLinkWithTarget:selector:")
	if !objc.Send[bool](window, objc.RegisterName("respondsToSelector:"), create) {
		return nil
	}
	tick := objc.RegisterName("islandDisplayTick:")
	pause := objc.RegisterName("setPaused:")
	targetClass, err := objc.RegisterClass("DynamicIslandAnimationTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: tick, Fn: func(_ objc.ID, _ objc.SEL, link objc.ID) {
			a.windowFramePending = a.advanceWindowFrame(time.Now())
			if !a.windowFramePending {
				objc.Send[objc.ID](link, pause, true)
			}
		}},
	})
	if err != nil {
		log.Printf("window display link: %v", err)
		return nil
	}
	target := objc.Send[objc.ID](objc.ID(targetClass), objc.RegisterName("new"))
	link := objc.Send[objc.ID](window, create, target, tick)
	objc.Send[objc.ID](target, objc.RegisterName("release"))
	if link == 0 {
		return nil
	}
	objc.Send[objc.ID](link, objc.RegisterName("retain"))
	objc.Send[objc.ID](link, pause, true)
	runLoop := objc.Send[objc.ID](objc.ID(objc.GetClass("NSRunLoop")), objc.RegisterName("mainRunLoop"))
	commonModes := objc.Send[objc.ID](objc.ID(objc.GetClass("NSString")), objc.RegisterName("stringWithUTF8String:"), "kCFRunLoopCommonModes")
	objc.Send[objc.ID](link, objc.RegisterName("addToRunLoop:forMode:"), runLoop, commonModes)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		objc.Send[objc.ID](link, objc.RegisterName("invalidate"))
		objc.Send[objc.ID](link, objc.RegisterName("release"))
	})
	return func() { objc.Send[objc.ID](link, pause, false) }
}

type cocoaWindowRect struct {
	x, y, width, height float64
}

var (
	cocoaFrame           = objc.RegisterName("frame")
	cocoaScreens         = objc.RegisterName("screens")
	cocoaFirstObject     = objc.RegisterName("firstObject")
	cocoaSetFrameDisplay = objc.RegisterName("setFrame:display:")
)

func setIslandWindowBounds(win *mygo.Window, bounds mygo.Rectangle, animated bool) {
	window := objc.ID(win.NativeHandle())
	content := objc.Send[objc.ID](window, cocoaContentView)
	views := objc.Send[objc.ID](content, cocoaSubviews)
	for i := range objc.Send[uint](views, cocoaCount) {
		view := objc.Send[objc.ID](views, cocoaObjectAt, i)
		if anchorMetalContents(objc.Send[objc.ID](view, cocoaLayer)) {
			break
		}
	}
	if !animated {
		win.SetBounds(bounds)
		return
	}
	// Keep AppKit from forcing a synchronous display at the new size.
	// MyGo's view resize event and display link schedule the paint.
	screens := objc.Send[objc.ID](objc.ID(objc.GetClass("NSScreen")), cocoaScreens)
	mainScreen := objc.Send[objc.ID](screens, cocoaFirstObject)
	screen := objc.Send[cocoaWindowRect](mainScreen, cocoaFrame)
	frame := cocoaWindowRect{
		x: float64(bounds.X), y: screen.height - float64(bounds.Y+bounds.Height),
		width: float64(bounds.Width), height: float64(bounds.Height),
	}
	objc.Send[objc.ID](window, cocoaSetFrameDisplay, frame, false)
}

var (
	cocoaContentView     = objc.RegisterName("contentView")
	cocoaSubviews        = objc.RegisterName("subviews")
	cocoaCount           = objc.RegisterName("count")
	cocoaObjectAt        = objc.RegisterName("objectAtIndex:")
	cocoaIsKindOf        = objc.RegisterName("isKindOfClass:")
	cocoaBounds          = objc.RegisterName("bounds")
	cocoaSetFrame        = objc.RegisterName("setFrame:")
	cocoaMask            = objc.RegisterName("autoresizingMask")
	cocoaSetMask         = objc.RegisterName("setAutoresizingMask:")
	cocoaLayer           = objc.RegisterName("layer")
	cocoaSublayers       = objc.RegisterName("sublayers")
	cocoaContentsFlipped = objc.RegisterName("contentsAreFlipped")
	cocoaSetGravity      = objc.RegisterName("setContentsGravity:")
	cocoaLayerMask       = objc.RegisterName("mask")
	cocoaSetLayerMask    = objc.RegisterName("setMask:")
	cocoaGeometryFlipped = objc.RegisterName("isGeometryFlipped")
	cocoaBlackColor      = objc.RegisterName("blackColor")
	cocoaCGColor         = objc.RegisterName("CGColor")
	cocoaSetColor        = objc.RegisterName("setBackgroundColor:")
	cocoaSetCorner       = objc.RegisterName("setCornerRadius:")
	cocoaBegin           = objc.RegisterName("begin")
	cocoaDisableActions  = objc.RegisterName("setDisableActions:")
	cocoaCommit          = objc.RegisterName("commit")
	cocoaRetain          = objc.RegisterName("retain")
	cocoaRelease         = objc.RegisterName("release")
)

func anchorMetalContents(parent objc.ID) bool {
	layers := objc.Send[objc.ID](parent, cocoaSublayers)
	metal := objc.GetClass("CAMetalLayer")
	for i := range objc.Send[uint](layers, cocoaCount) {
		layer := objc.Send[objc.ID](layers, cocoaObjectAt, i)
		if metal == 0 || !objc.Send[bool](layer, cocoaIsKindOf, metal) {
			continue
		}
		// Keep the last drawable at its original scale and aligned to the
		// island's top center until MyGo presents one at the new size.
		gravity := "top"
		if objc.Send[bool](layer, cocoaContentsFlipped) {
			gravity = "bottom"
		}
		value := objc.Send[objc.ID](objc.ID(objc.GetClass("NSString")), objc.RegisterName("stringWithUTF8String:"), gravity)
		objc.Send[objc.ID](layer, cocoaSetGravity, value)
		return true
	}
	return false
}

func islandWindowMaterial(content objc.ID) (view, radiusTarget objc.ID) {
	views := objc.Send[objc.ID](content, cocoaSubviews)
	glass, visualEffect := objc.GetClass("NSGlassEffectView"), objc.GetClass("NSVisualEffectView")
	for i := range objc.Send[uint](views, cocoaCount) {
		effect := objc.Send[objc.ID](views, cocoaObjectAt, i)
		if glass != 0 && objc.Send[bool](effect, cocoaIsKindOf, glass) {
			return effect, effect
		}
		if objc.Send[bool](effect, cocoaIsKindOf, visualEffect) {
			return effect, objc.Send[objc.ID](effect, cocoaLayer)
		}
	}
	return 0, 0
}

func setIslandWindowCornerRadius(win *mygo.Window, radius float32) {
	content := objc.Send[objc.ID](objc.ID(win.NativeHandle()), cocoaContentView)
	_, material := islandWindowMaterial(content)
	transaction := objc.ID(objc.GetClass("CATransaction"))
	objc.Send[objc.ID](transaction, cocoaBegin)
	objc.Send[objc.ID](transaction, cocoaDisableActions, true)
	objc.Send[objc.ID](objc.Send[objc.ID](content, cocoaLayer), cocoaSetCorner, float64(radius))
	objc.Send[objc.ID](material, cocoaSetCorner, float64(radius))
	objc.Send[objc.ID](transaction, cocoaCommit)
}

func startTransitionCanvas(win *mygo.Window) transitionCanvas {
	content := objc.Send[objc.ID](objc.ID(win.NativeHandle()), cocoaContentView)
	layer := objc.Send[objc.ID](content, cocoaLayer)
	effect, material := islandWindowMaterial(content)
	if layer == 0 || effect == 0 {
		return transitionCanvas{}
	}
	mask := objc.Send[uint](effect, cocoaMask)
	// Mask both the glass and the last submitted Metal frame. They can
	// reach the compositor on different refreshes during a transition.
	originalMask := objc.Send[objc.ID](layer, cocoaLayerMask)
	objc.Send[objc.ID](originalMask, cocoaRetain)
	clip := objc.Send[objc.ID](objc.ID(objc.GetClass("CALayer")), cocoaLayer)
	objc.Send[objc.ID](clip, cocoaRetain)
	black := objc.Send[objc.ID](objc.ID(objc.GetClass("NSColor")), cocoaBlackColor)
	objc.Send[objc.ID](clip, cocoaSetColor, objc.Send[objc.ID](black, cocoaCGColor))
	transaction := objc.ID(objc.GetClass("CATransaction"))
	flipped := objc.Send[bool](layer, cocoaGeometryFlipped)
	objc.Send[objc.ID](effect, cocoaSetMask, 0)
	return transitionCanvas{
		bounds: win.Bounds(),
		reshape: func(r mygo.Rectangle, height int) {
			rect := cocoaWindowRect{
				x: float64(r.X), y: float64(height - r.Y - r.Height),
				width: float64(r.Width), height: float64(r.Height),
			}
			radius := float64(islandCornerRadius(float32(r.Height)))
			objc.Send[objc.ID](transaction, cocoaBegin)
			objc.Send[objc.ID](transaction, cocoaDisableActions, true)
			objc.Send[objc.ID](layer, cocoaSetCorner, radius)
			objc.Send[objc.ID](material, cocoaSetCorner, radius)
			objc.Send[objc.ID](clip, cocoaSetCorner, radius)
			objc.Send[objc.ID](effect, cocoaSetFrame, rect)
			if flipped {
				rect.y = float64(r.Y)
			}
			objc.Send[objc.ID](clip, cocoaSetFrame, rect)
			objc.Send[objc.ID](layer, cocoaSetLayerMask, clip)
			objc.Send[objc.ID](transaction, cocoaCommit)
		},
		restore: func() {
			objc.Send[objc.ID](transaction, cocoaBegin)
			objc.Send[objc.ID](transaction, cocoaDisableActions, true)
			objc.Send[objc.ID](effect, cocoaSetFrame, objc.Send[cocoaWindowRect](content, cocoaBounds))
			objc.Send[objc.ID](effect, cocoaSetMask, mask)
			objc.Send[objc.ID](layer, cocoaSetLayerMask, originalMask)
			objc.Send[objc.ID](originalMask, cocoaRelease)
			objc.Send[objc.ID](clip, cocoaRelease)
			objc.Send[objc.ID](transaction, cocoaCommit)
		},
	}
}
