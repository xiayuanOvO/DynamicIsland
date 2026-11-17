package main

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	gdi32              = syscall.NewLazyDLL("gdi32.dll")
	getWindowRect      = user32.NewProc("GetWindowRect")
	getCursorPos       = user32.NewProc("GetCursorPos")
	getDPIForWindow    = user32.NewProc("GetDpiForWindow")
	setWindowRgn       = user32.NewProc("SetWindowRgn")
	createRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	deleteObject       = gdi32.NewProc("DeleteObject")
	lastWindowShape    windowShape
)

// Resizing moves the window under a stationary pointer without reliably
// refreshing the UI's cached hover coordinates.
func pointerOverWindow(win *mygo.Window, fallback bool) bool {
	var point struct{ x, y int32 }
	var rect struct{ left, top, right, bottom int32 }
	if ok, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return fallback
	}
	if ok, _, _ := getWindowRect.Call(win.NativeHandle(), uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return fallback
	}
	return point.x >= rect.left && point.x < rect.right && point.y >= rect.top && point.y < rect.bottom
}

type windowShape struct {
	hwnd          uintptr
	width, height int32
	diameter      uintptr
}

// Shape the native window to match the drawn capsule, including its hit area.
func roundWindow(win *mygo.Window, radius int) {
	mygo.RunOnMain(func() {
		hwnd := win.NativeHandle()
		if hwnd == 0 {
			return
		}
		var rect struct{ left, top, right, bottom int32 }
		if ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
			return
		}
		dpi, _, _ := getDPIForWindow.Call(hwnd)
		if dpi == 0 {
			dpi = 96
		}
		diameter := uintptr(radius*2) * dpi / 96
		shape := windowShape{hwnd, rect.right - rect.left, rect.bottom - rect.top, diameter}
		if shape == lastWindowShape {
			return
		}
		region, _, _ := createRoundRectRgn.Call(0, 0,
			uintptr(rect.right-rect.left+1), uintptr(rect.bottom-rect.top+1), diameter, diameter)
		if region == 0 {
			return
		}
		// SetWindowRgn sends window-position messages, so cache first to
		// avoid applying the same region again from a nested resize event.
		lastWindowShape = shape
		// On success Windows owns the region; on failure we must release it.
		if ok, _, _ := setWindowRgn.Call(hwnd, region, 1); ok == 0 {
			deleteObject.Call(region)
			lastWindowShape = windowShape{}
		}
	})
}
