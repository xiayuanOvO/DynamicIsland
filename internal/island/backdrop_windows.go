package island

import (
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/go-ole/go-ole"
)

// Windows.UI.Composition uses one COM interface per version of a runtime
// class. Slots below are from the Windows SDK's windows.ui.composition.h.
//
//go:uintptrescapes
func compositionCall(object *ole.IUnknown, slot int, args ...uintptr) bool {
	method := unsafe.Slice((*uintptr)(unsafe.Pointer(object.RawVTable)), slot+1)[slot]
	hr, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)...)
	return int32(hr) >= 0
}

// Use a child HWND for the system blur. Its visual sits beneath MyGo's UI
// target and follows the parent window's position and visibility.
func startWindowBackdrop(win *mygo.Window) func(width, height, radius float32) {
	if runtime.GOARCH != "amd64" {
		// The by-value Vector2 and DispatcherQueueOptions calls below use the x64 ABI.
		return nil
	}
	parent := win.NativeHandle()
	const dwmwaUseHostBackdropBrush = 17
	enable := int32(1)
	hr, _, _ := dwmSetWindowAttribute.Call(parent, dwmwaUseHostBackdropBrush, uintptr(unsafe.Pointer(&enable)), unsafe.Sizeof(enable))
	if int32(hr) < 0 {
		return nil
	}

	const (
		wsExNoRedirectionBitmap = 0x00200000
		wsExTransparent         = 0x00000020
		wsChild                 = 0x40000000
		wsVisible               = 0x10000000
		wsDisabled              = 0x08000000
	)
	class, _ := syscall.UTF16PtrFromString("Static")
	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(wsExNoRedirectionBitmap|wsExTransparent,
		uintptr(unsafe.Pointer(class)), 0, wsChild|wsVisible|wsDisabled, 0, 0, 0, 0, parent, 0, 0, 0)
	if hwnd == 0 {
		return nil
	}
	installed := false
	defer func() {
		if !installed {
			user32.NewProc("DestroyWindow").Call(hwnd)
		}
	}()

	// MyGo already owns the UI thread's STA and message loop.
	combase := syscall.NewLazyDLL("combase.dll")
	hr, _, _ = combase.NewProc("RoInitialize").Call(0) // RO_INIT_SINGLETHREADED
	if int32(hr) < 0 {
		return nil
	}
	uninitialize := combase.NewProc("RoUninitialize")
	createQueue := syscall.NewLazyDLL("CoreMessaging.dll").NewProc("CreateDispatcherQueueController")
	options := [...]uint32{12, 2, 0} // current thread, reuse its apartment
	var queue *ole.IUnknown
	hr, _, _ = createQueue.Call(uintptr(unsafe.Pointer(&options)), uintptr(unsafe.Pointer(&queue)))
	if int32(hr) < 0 {
		uninitialize.Call()
		return nil
	}
	objects := []*ole.IUnknown{queue}
	release := func() {
		for i := len(objects) - 1; i >= 0; i-- {
			objects[i].Release()
		}
		uninitialize.Call()
	}
	compositor, err := ole.RoActivateInstance("Windows.UI.Composition.Compositor")
	if err != nil {
		release()
		return nil
	}
	objects = append(objects, &compositor.IUnknown)
	query := func(object *ole.IUnknown, iid string) *ole.IUnknown {
		var result *ole.IUnknown
		if object != nil && object.PutQueryInterface(ole.NewGUID(iid), &result) == nil {
			objects = append(objects, result)
		}
		return result
	}
	// The COM out parameter is passed through a variadic argument slice.
	var output *ole.IUnknown
	var pin runtime.Pinner
	pin.Pin(&output)
	defer pin.Unpin()
	create := func(object *ole.IUnknown, slot int, args ...uintptr) *ole.IUnknown {
		if object == nil || !compositionCall(object, slot, append(args, uintptr(unsafe.Pointer(&output)))...) {
			return nil
		}
		objects = append(objects, output)
		return output
	}
	base := &compositor.IUnknown
	desktop := query(base, "{29E691FA-4567-4DCA-B319-D0F207EB6807}")        // ICompositorDesktopInterop
	windowTarget := create(desktop, 3, hwnd, 1)                             // CreateDesktopWindowTarget
	target := query(windowTarget, "{A1BEA8BA-D726-4663-8129-6B5E7927FFA6}") // ICompositionTarget
	compositor1 := query(base, "{B403CA50-7F8C-4E83-985F-CC45060036D8}")
	compositor3 := query(base, "{C9DD8EF0-6EB1-4E3C-A658-675D9C64D4AB}")
	compositor5 := query(base, "{48EA31AD-7FCD-4076-A79C-90CC4B852C9B}")
	compositor6 := query(base, "{7A38B2BD-CEC8-4EEB-830F-D8D07AEDEBC3}")
	sprite := create(compositor1, 22)                                    // CreateSpriteVisual
	visual := query(sprite, "{117E202D-A859-4C89-873B-C2AA566788E3}")    // IVisual
	backdrop := create(compositor3, 6)                                   // CreateHostBackdropBrush
	brush := query(backdrop, "{AB0D7608-30C0-40E9-B568-B60A6BD1FB46}")   // ICompositionBrush
	rounded := create(compositor5, 20)                                   // CreateRoundedRectangleGeometry
	geometry := query(rounded, "{E985217C-6A17-4207-ABD8-5FD3DD612A9D}") // ICompositionGeometry
	if target == nil || visual == nil || brush == nil || geometry == nil {
		release()
		return nil
	}
	geometricClip := create(compositor6, 7, uintptr(unsafe.Pointer(geometry)))       // CreateGeometricClipWithGeometry
	clip := query(geometricClip, "{1CCD2A52-CFC7-4ACE-9983-146BB8EB6A3C}")           // ICompositionClip
	if clip == nil || !compositionCall(sprite, 7, uintptr(unsafe.Pointer(brush))) || // put_Brush
		!compositionCall(visual, 11, 1) || // put_BorderMode(Soft)
		!compositionCall(visual, 15, uintptr(unsafe.Pointer(clip))) || // put_Clip
		!compositionCall(target, 7, uintptr(unsafe.Pointer(visual))) { // put_Root
		release()
		return nil
	}
	installed = true
	mygo.App.OnQuit(release)
	return func(width, height, radius float32) {
		const swpNoMove, swpNoZOrder, swpNoActivate = 0x0002, 0x0004, 0x0010
		setWindowPos.Call(hwnd, 0, 0, 0, uintptr(width), uintptr(height), swpNoMove|swpNoZOrder|swpNoActivate)
		vector := func(x, y float32) uintptr {
			return uintptr(uint64(math.Float32bits(x)) | uint64(math.Float32bits(y))<<32)
		}
		size := vector(width, height)
		compositionCall(rounded, 7, vector(radius, radius)) // put_CornerRadius
		compositionCall(rounded, 11, size)                  // put_Size
		compositionCall(visual, 36, size)                   // put_Size
	}
}
