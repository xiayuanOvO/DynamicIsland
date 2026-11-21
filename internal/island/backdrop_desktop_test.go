//go:build windows && amd64 && desktoptest

package island

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMain(m *testing.M) {
	if code := m.Run(); code != 0 {
		os.Exit(code)
	}
	if err := checkDesktopBackdrop(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Run on a Windows x64 desktop with transparency enabled:
// go test -count=1 -tags desktoptest ./internal/island
func checkDesktopBackdrop() error {
	result := make(chan error, 1)
	mygo.App.WhenReady(func() {
		area := mygo.Screen.PrimaryDisplay().WorkArea
		x, y := area.X+area.Width/2, area.Y+200
		mygo.NewWindow(mygo.WindowOptions{
			X: x - 360, Y: y - 70, Width: 720, Height: 350,
			Frameless: true, AlwaysOnTop: true, DisableShadow: true,
			Content: ui.View(func(c *ui.Context) {
				ui.Row(c).Fill().Children(func() {
					ui.Box(c).Width(360).FillHeight().Background(ui.Hex("#0c1b39"))
					ui.Box(c).Width(360).FillHeight().Background(ui.Hex("#621406"))
				})
			}),
		})
		time.AfterFunc(800*time.Millisecond, func() {
			mygo.RunOnMain(func() {
				backdrop := false
				win := mygo.NewWindow(mygo.WindowOptions{
					X: x - 100, Y: y, Width: 200, Height: compactHeight,
					Frameless: true, Panel: true, AlwaysOnTop: true, Hidden: true,
					Transparent: true, DisableShadow: true, BackgroundColor: "#00000000",
					Content: ui.View(func(c *ui.Context) {
						c.Root().Background(ui.Transparent)
						_, height := c.Size()
						surface := ui.Box(c).Fill().Radius(islandCornerRadius(height)).Clip()
						glassSurface(surface, backdrop, false, false)
					}),
				})
				configureFloatingWindow(win)
				backdrop = configureWindowGlass(win, windowRadius)
				win.ShowInactive()
				win.Invalidate()
				frames := [...]mygo.Size{
					{Width: 200, Height: idleCapsuleHeight},
					{Width: 200, Height: compactHeight},
					{Width: panelWidth, Height: 170},
					{Width: compactHeight, Height: compactHeight},
				}
				go func() {
					defer mygo.App.Quit()
					for _, size := range frames {
						win.SetBounds(mygo.Rectangle{X: x - size.Width/2, Y: y, Width: size.Width, Height: size.Height})
						time.Sleep(1100 * time.Millisecond)
						done := make(chan error, 1)
						mygo.RunOnMain(func() { done <- checkBackdropPixels(win.NativeHandle()) })
						if err := <-done; err != nil {
							result <- fmt.Errorf("%dx%d: %w", size.Width, size.Height, err)
							return
						}
					}
					result <- nil
				}()
			})
		})
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	default:
		return fmt.Errorf("the desktop test closed before checking the backdrop")
	}
}

func checkBackdropPixels(hwnd uintptr) error {
	var bounds struct{ left, top, right, bottom int32 }
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
	dc, _, _ := user32.NewProc("GetDC").Call(0)
	defer user32.NewProc("ReleaseDC").Call(0, dc)
	getPixel := gdi32.NewProc("GetPixel")
	pixel := func(x, y int32) uint32 {
		value, _, _ := getPixel.Call(dc, uintptr(x), uintptr(y))
		return uint32(value)
	}
	left, right := bounds.left+1, bounds.right-2
	top, bottom := bounds.top+1, bounds.bottom-2
	for _, x := range []int32{left, right} {
		background := pixel(x, bounds.top-10)
		for _, y := range []int32{top, bottom} {
			if color := pixel(x, y); color != background || color == 0xffffffff {
				return fmt.Errorf("blur leaked outside the corner at (%d, %d): got %#x, background %#x", x, y, color, background)
			}
		}
	}

	// The background has a sharp color boundary. Inside the island it
	// should turn into a gradual change spread across multiple pixels.
	mid := (left + right) / 2
	row := (top + bottom) / 2
	point := uintptr(uint64(uint32(mid)) | uint64(uint32(row))<<32)
	hit, _, _ := user32.NewProc("WindowFromPoint").Call(point)
	const gaRoot = 2
	if root, _, _ := user32.NewProc("GetAncestor").Call(hit, gaRoot); root != hwnd {
		return fmt.Errorf("the island no longer receives input at its center")
	}
	if pixel(mid-22, bounds.top-10) == pixel(mid+24, bounds.top-10) {
		return fmt.Errorf("the two-color test background is obscured")
	}
	span := min(int32(24), (right-left)/2-5)
	chroma := func(color uint32) float64 {
		return float64(color&255) - float64(color>>16&255)
	}
	previous := chroma(pixel(mid-span, row))
	start, largestStep := previous, 0.0
	for x := mid - span + 1; x <= mid+span; x++ {
		current := chroma(pixel(x, row))
		largestStep = max(largestStep, math.Abs(current-previous))
		previous = current
	}
	if math.Abs(previous-start) < 6 || largestStep > 9 {
		return fmt.Errorf("background transition is missing or sharp, total change %.0f, largest pixel step %.0f", previous-start, largestStep)
	}
	return nil
}
