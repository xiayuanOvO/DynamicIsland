package main

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	gdi32                 = syscall.NewLazyDLL("gdi32.dll")
	getForegroundWindow   = user32.NewProc("GetForegroundWindow")
	getShellWindow        = user32.NewProc("GetShellWindow")
	isZoomed              = user32.NewProc("IsZoomed")
	monitorFromWindow     = user32.NewProc("MonitorFromWindow")
	getMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	dwmGetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	getWindowRect         = user32.NewProc("GetWindowRect")
	getCursorPos          = user32.NewProc("GetCursorPos")
	getAsyncKeyState      = user32.NewProc("GetAsyncKeyState")
	getSystemMetrics      = user32.NewProc("GetSystemMetrics")
	getDPIForWindow       = user32.NewProc("GetDpiForWindow")
	setWindowRgn          = user32.NewProc("SetWindowRgn")
	createRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	deleteObject          = gdi32.NewProc("DeleteObject")
	lastWindowShape       windowShape
)

func primaryMouseDown() bool {
	const leftButton, rightButton, swapButton = 0x01, 0x02, 23
	button := uintptr(leftButton)
	if swapped, _, _ := getSystemMetrics.Call(swapButton); swapped != 0 {
		button = rightButton
	}
	state, _, _ := getAsyncKeyState.Call(button)
	return state&0x8000 != 0
}

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

func foregroundFullscreen() bool {
	hwnd, _, _ := getForegroundWindow.Call()
	shell, _, _ := getShellWindow.Call()
	if hwnd == 0 || hwnd == shell {
		return false
	}
	if maximized, _, _ := isZoomed.Call(hwnd); maximized != 0 {
		return false
	}
	monitor, _, _ := monitorFromWindow.Call(hwnd, 0)
	if monitor == 0 {
		return false
	}
	type rect struct{ left, top, right, bottom int32 }
	var info struct {
		size          uint32
		monitor, work rect
		flags         uint32
	}
	info.size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false
	}
	var window rect
	const extendedFrameBounds = 9
	status, _, _ := dwmGetWindowAttribute.Call(hwnd, extendedFrameBounds, uintptr(unsafe.Pointer(&window)), unsafe.Sizeof(window))
	if int32(status) < 0 {
		if ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&window))); ok == 0 {
			return false
		}
	}
	return windowCoversDisplay(
		mygo.Rectangle{X: int(window.left), Y: int(window.top), Width: int(window.right - window.left), Height: int(window.bottom - window.top)},
		mygo.Rectangle{X: int(info.monitor.left), Y: int(info.monitor.top), Width: int(info.monitor.right - info.monitor.left), Height: int(info.monitor.bottom - info.monitor.top)},
	)
}
