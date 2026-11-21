package island

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

func TestIslandMetalContentsAnchoredAtTopCenter(t *testing.T) {
	quartzCore, err := purego.Dlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore", purego.RTLD_LAZY)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(quartzCore)
	pool := objc.Send[objc.ID](objc.ID(objc.GetClass("NSAutoreleasePool")), objc.RegisterName("new"))
	defer objc.Send[objc.ID](pool, objc.RegisterName("drain"))
	parent := objc.Send[objc.ID](objc.ID(objc.GetClass("CALayer")), cocoaLayer)
	metal := objc.Send[objc.ID](objc.ID(objc.GetClass("CAMetalLayer")), cocoaLayer)
	objc.Send[objc.ID](parent, objc.RegisterName("addSublayer:"), metal)

	for _, test := range []struct {
		flipped bool
		gravity string
	}{{false, "top"}, {true, "bottom"}} {
		objc.Send[objc.ID](parent, objc.RegisterName("setGeometryFlipped:"), test.flipped)
		if !anchorMetalContents(parent) {
			t.Fatal("Metal drawing layer was not found")
		}
		value := objc.Send[objc.ID](metal, objc.RegisterName("contentsGravity"))
		if got := objc.Send[string](value, objc.RegisterName("UTF8String")); got != test.gravity {
			t.Errorf("flipped=%v: contents gravity = %q, want %q", test.flipped, got, test.gravity)
		}
	}
}
