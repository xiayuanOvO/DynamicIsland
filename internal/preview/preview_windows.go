package preview

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	shell32        = syscall.NewLazyDLL("shell32.dll")
	ole32          = syscall.NewLazyDLL("ole32.dll")
	gdi32          = syscall.NewLazyDLL("gdi32.dll")
	createItem     = shell32.NewProc("SHCreateItemFromParsingName")
	coInitialize   = ole32.NewProc("CoInitializeEx")
	coUninitialize = ole32.NewProc("CoUninitialize")
	getDIBits      = gdi32.NewProc("GetDIBits")
	createDC       = gdi32.NewProc("CreateCompatibleDC")
	deleteDC       = gdi32.NewProc("DeleteDC")
	deleteObject   = gdi32.NewProc("DeleteObject")
	imageFactoryID = syscall.GUID{
		Data1: 0xbcc18b79, Data2: 0xba16, Data3: 0x442f,
		Data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b},
	}
)

type imageFactory struct{ vtable *[4]uintptr }

type bitmapHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}

type bitmapInfo struct {
	bitmapHeader
	Colors [256]uint32
}

func systemImage(path string, thumbnail bool) (image.Image, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Each worker owns its COM apartment and releases it on this thread.
	if hr, _, _ := coInitialize.Call(0, 2); int32(hr) < 0 {
		return nil, fmt.Errorf("CoInitializeEx: %#x", uint32(hr))
	}
	defer coUninitialize.Call()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var factory *imageFactory
	if hr, _, _ := createItem.Call(uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&imageFactoryID)), uintptr(unsafe.Pointer(&factory))); int32(hr) < 0 || factory == nil {
		return nil, fmt.Errorf("SHCreateItemFromParsingName: %#x", uint32(hr))
	}
	self := uintptr(unsafe.Pointer(factory))
	defer syscall.SyscallN(factory.vtable[2], self) // IUnknown.Release
	size, flags := uintptr(96), uintptr(0x4)        // SIIGBF_ICONONLY
	if thumbnail {
		size, flags = Size, 0x8 // SIIGBF_THUMBNAILONLY: never substitute an icon.
	}
	var bitmap uintptr
	var hr uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		hr, _, _ = syscall.SyscallN(factory.vtable[3], self, size|size<<32, flags, uintptr(unsafe.Pointer(&bitmap)))
	} else {
		hr, _, _ = syscall.SyscallN(factory.vtable[3], self, size, size, flags, uintptr(unsafe.Pointer(&bitmap)))
	}
	if int32(hr) < 0 || bitmap == 0 {
		return nil, fmt.Errorf("IShellItemImageFactory.GetImage: %#x", uint32(hr))
	}
	defer deleteObject.Call(bitmap)
	return bitmapImage(bitmap)
}

func bitmapImage(bitmap uintptr) (image.Image, error) {
	dc, _, _ := createDC.Call(0)
	if dc == 0 {
		return nil, fmt.Errorf("cannot create bitmap device context")
	}
	defer deleteDC.Call(dc)
	info := bitmapInfo{bitmapHeader: bitmapHeader{Size: uint32(unsafe.Sizeof(bitmapHeader{}))}}
	if ok, _, _ := getDIBits.Call(dc, bitmap, 0, 0, 0, uintptr(unsafe.Pointer(&info)), 0); ok == 0 {
		return nil, fmt.Errorf("cannot query thumbnail bitmap")
	}
	w, h := int(info.Width), int(info.Height)
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 || w > Size || h > Size {
		return nil, fmt.Errorf("unexpected thumbnail dimensions: %dx%d", w, h)
	}
	opaque := info.BitCount < 32
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	info.Height, info.Planes, info.BitCount = -int32(h), 1, 32
	info.Compression, info.SizeImage, info.ClrUsed, info.ClrImportant = 0, 0, 0, 0
	if lines, _, _ := getDIBits.Call(dc, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&img.Pix[0])),
		uintptr(unsafe.Pointer(&info)), 0); lines != uintptr(h) {
		return nil, fmt.Errorf("cannot read thumbnail bitmap")
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
		if opaque {
			img.Pix[i+3] = 255
		}
	}
	return img, nil
}
