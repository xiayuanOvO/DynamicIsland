package battery

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getSystemPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

func Read(context.Context) []Reading {
	var status struct {
		acLine, flags, percent, saver byte
		remaining, fullLifetime       uint32
	}
	ok, _, _ := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 || status.percent > 100 || (status.flags != 255 && status.flags&128 != 0) {
		return nil
	}
	return []Reading{{ID: "computer", Name: "本机", Kind: Computer, Percent: int(status.percent), Charging: status.flags != 255 && status.flags&8 != 0}}
}
