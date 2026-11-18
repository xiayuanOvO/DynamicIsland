package main

import "github.com/ebitengine/purego/objc"

var pressedMouseButtons = objc.RegisterName("pressedMouseButtons")

func primaryMouseDown() bool {
	return objc.Send[uint](objc.ID(objc.GetClass("NSEvent")), pressedMouseButtons)&1 != 0
}
