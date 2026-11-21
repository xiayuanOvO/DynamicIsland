package battery

import (
	"runtime"
	"slices"
)

const Supported = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

const UnknownPercent = -1

type Kind uint8

const (
	Other Kind = iota
	Computer
	Mouse
	Keyboard
	Trackpad
	Headphones
)

type Part uint8

const (
	Whole Part = iota
	Left
	Right
	Case
)

type Reading struct {
	ID, Name string
	Kind     Kind
	Part     Part
	Percent  int
	Charging bool
}

// Snapshot contains the readings from each battery source. A nil slice
// means there was no usable result. A non-nil empty slice means a successful
// read found no batteries.
type Snapshot struct {
	computer  []Reading
	bluetooth []Reading
}

func (s Snapshot) Merge(cached []Reading) []Reading {
	if s.computer == nil && s.bluetooth == nil {
		return cached
	}
	computer, bluetooth := s.computer, s.bluetooth
	for _, reading := range cached {
		switch {
		case reading.Kind == Computer && s.computer == nil:
			computer = append(computer, reading)
		case reading.Kind != Computer && s.bluetooth == nil:
			bluetooth = append(bluetooth, reading)
		}
	}
	return slices.Concat(computer, bluetooth)
}
