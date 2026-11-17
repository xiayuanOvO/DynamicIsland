package audiolevel

import (
	"context"
	"encoding/binary"
	"math"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func captureSystemLevel(ctx context.Context, report func(*[fftSize]float32, int)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// S_FALSE means this thread was already initialized and still needs
		// a matching CoUninitialize.
		if code, ok := err.(*ole.OleError); !ok || code.Code() != 1 {
			return
		}
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator) != nil {
		return
	}
	defer enumerator.Release()
	var device *wca.IMMDevice
	if enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &device) != nil {
		return
	}
	defer device.Release()
	var client *wca.IAudioClient
	// go-wca v0.3.0 passes the address of dwClsCtx instead of its value.
	status, _, _ := syscall.SyscallN(device.VTable().Activate,
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(wca.IID_IAudioClient)),
		uintptr(wca.CLSCTX_ALL), 0, uintptr(unsafe.Pointer(&client)))
	if int32(status) < 0 {
		return
	}
	defer client.Release()

	var mix *wca.WAVEFORMATEX
	if client.GetMixFormat(&mix) != nil {
		return
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(mix)))
	format := mix.WFormatTag
	if format == 0xfffe && mix.CbSize >= 22 {
		// WAVEFORMATEXTENSIBLE stores the subtype GUID at byte offset 24.
		format = *(*uint16)(unsafe.Add(unsafe.Pointer(mix), 24))
	}
	rate, channels := int(mix.NSamplesPerSec), int(mix.NChannels)
	frameSize, width := int(mix.NBlockAlign), int(mix.WBitsPerSample/8)
	if rate < 8000 || rate > 384000 || channels == 0 || frameSize != channels*width {
		return
	}
	if !(format == 3 && width == 4 || format == 1 && width >= 1 && width <= 4) {
		return
	}
	const bufferDuration = 1_000_000 // 100 ms in 100-nanosecond units.
	if client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK, bufferDuration, 0, mix, nil) != nil {
		return
	}
	var capture *wca.IAudioCaptureClient
	if client.GetService(wca.IID_IAudioCaptureClient, &capture) != nil {
		return
	}
	defer capture.Release()
	if client.Start() != nil {
		return
	}
	defer client.Stop()

	var ring, samples [fftSize]float32
	var lastPacket time.Time
	index := 0
	tick := time.NewTicker(SampleInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			for {
				var frames uint32
				if capture.GetNextPacketSize(&frames) != nil {
					return
				}
				if frames == 0 {
					break
				}
				var data *byte
				var flags uint32
				if capture.GetBuffer(&data, &frames, &flags, nil, nil) != nil {
					return
				}
				if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 {
					for range frames {
						ring[index] = 0
						index = (index + 1) % fftSize
					}
				} else {
					packet := unsafe.Slice(data, int(frames)*frameSize)
					for frame := 0; frame < len(packet); frame += frameSize {
						var mono float32
						for offset := frame; offset < frame+frameSize; offset += width {
							b := packet[offset : offset+width]
							switch {
							case format == 3:
								mono += math.Float32frombits(binary.LittleEndian.Uint32(b))
							case width == 1:
								mono += float32(int(b[0])-128) / 128
							case width == 2:
								mono += float32(int16(binary.LittleEndian.Uint16(b))) / 32768
							case width == 3:
								raw := uint32(b[0])<<8 | uint32(b[1])<<16 | uint32(b[2])<<24
								mono += float32(int32(raw)) / 2147483648
							case width == 4:
								mono += float32(int32(binary.LittleEndian.Uint32(b))) / 2147483648
							}
						}
						ring[index] = mono / float32(channels)
						index = (index + 1) % fftSize
					}
				}
				if capture.ReleaseBuffer(frames) != nil {
					return
				}
				lastPacket = now
			}
			if now.Sub(lastPacket) > 100*time.Millisecond {
				clear(ring[:])
			}
			copy(samples[:], ring[index:])
			copy(samples[fftSize-index:], ring[:index])
			report(&samples, rate)
		}
	}
}
