package battery

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getSystemPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

func Read(context.Context) Snapshot {
	const charging, noBattery, unknownStatus = 8, 128, 255
	var status struct {
		acLine, flags, percent, saver byte
		remaining, fullLifetime       uint32
	}
	ok, _, _ := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return Snapshot{}
	}
	if status.flags != unknownStatus && status.flags&noBattery != 0 {
		return Snapshot{computer: []Reading{}}
	}
	if status.percent > 100 {
		return Snapshot{}
	}
	return Snapshot{computer: []Reading{{
		ID: "computer", Name: "本机", Kind: Computer, Percent: int(status.percent),
		Charging: status.flags != unknownStatus && status.flags&charging != 0,
	}}}
}

func ReadIncremental(ctx context.Context, _ func(Snapshot)) Snapshot { return Read(ctx) }
