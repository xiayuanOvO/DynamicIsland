package battery

import "runtime"

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
