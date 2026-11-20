package island

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	gdi32                 = syscall.NewLazyDLL("gdi32.dll")
	getWindowLong         = user32.NewProc("GetWindowLongW")
	setWindowLong         = user32.NewProc("SetWindowLongW")
	setWindowPos          = user32.NewProc("SetWindowPos")
	getForegroundWindow   = user32.NewProc("GetForegroundWindow")
	getShellWindow        = user32.NewProc("GetShellWindow")
	isZoomed              = user32.NewProc("IsZoomed")
	monitorFromWindow     = user32.NewProc("MonitorFromWindow")
	getMonitorInfo        = user32.NewProc("GetMonitorInfoW")
	dwmapi                = syscall.NewLazyDLL("dwmapi.dll")
	dwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
	dwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	getWindowRect         = user32.NewProc("GetWindowRect")
	getCursorPos          = user32.NewProc("GetCursorPos")
	getAsyncKeyState      = user32.NewProc("GetAsyncKeyState")
	getSystemMetrics      = user32.NewProc("GetSystemMetrics")
	getDPIForWindow       = user32.NewProc("GetDpiForWindow")
	setWindowRgn          = user32.NewProc("SetWindowRgn")
	createRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	deleteObject          = gdi32.NewProc("DeleteObject")
)

func configureFloatingWindow(win *mygo.Window) {
	hwnd := win.NativeHandle()
	// MyGo's frameless panels keep WS_CAPTION. Give these desktop overlays
	// a popup frame while preserving their ownership and input styles.
	const (
		gwlStyle        = ^uintptr(15) // GWL_STYLE (-16)
		wsCaption       = 0x00c00000
		wsPopup         = 0x80000000
		swpFrameChanged = 0x0020
		swpNoSize       = 0x0001
		swpNoMove       = 0x0002
		swpNoZOrder     = 0x0004
		swpNoActivate   = 0x0010
	)
	style, _, _ := getWindowLong.Call(hwnd, gwlStyle)
	if style != 0 {
		setWindowLong.Call(hwnd, gwlStyle, style&^wsCaption|wsPopup)
		setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpFrameChanged|swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate)
	}

	// Clear the extended frame and backdrop for transparent overlays.
	win.SetVibrancy(mygo.VibrancyNone)
	const dwmwaNCRenderingPolicy, dwmwaWindowCornerPreference, dwmwaBorderColor = 2, 33, 34
	disabled := int32(1) // DWMNCRP_DISABLED
	dwmSetWindowAttribute.Call(hwnd, dwmwaNCRenderingPolicy, uintptr(unsafe.Pointer(&disabled)), unsafe.Sizeof(disabled))
	// MyGo requests DWMWCP_ROUND for every panel, even a shadowless text
	// overlay. Opt out of the system's rounded outline and drop shadow.
	noCorners := int32(1) // DWMWCP_DONOTROUND
	dwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&noCorners)), unsafe.Sizeof(noCorners))
	none := uint32(0xfffffffe) // DWMWA_COLOR_NONE
	dwmSetWindowAttribute.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&none)), unsafe.Sizeof(none))
}

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

func configureWindowGlass(win *mygo.Window, radius int) {
	win.SetVibrancy(mygo.VibrancyAcrylic)
	var lastWindowShape [3]int32
	update := func() {
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
		shape := [3]int32{rect.right - rect.left, rect.bottom - rect.top, int32(diameter)}
		if shape == lastWindowShape {
			return
		}
		// The window region clips both the DWM backdrop and the hit area.
		region, _, _ := createRoundRectRgn.Call(0, 0, uintptr(shape[0]+1), uintptr(shape[1]+1), diameter, diameter)
		if region == 0 {
			return
		}
		// SetWindowRgn sends window-position messages, so cache first to
		// avoid applying the same region again from a nested resize event.
		lastWindowShape = shape
		// On success Windows owns the region; on failure we must release it.
		if ok, _, _ := setWindowRgn.Call(hwnd, region, 1); ok == 0 {
			deleteObject.Call(region)
			lastWindowShape = [3]int32{}
		}
	}
	win.OnResize(func() { mygo.RunOnMain(update) })
	update()
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
