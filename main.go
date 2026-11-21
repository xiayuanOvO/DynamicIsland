package main

import (
	_ "embed"

	"dynamicisland/internal/island"
)

//go:embed resources/icon.png
var appIcon []byte

func main() {
	island.Run(appIcon)
}
