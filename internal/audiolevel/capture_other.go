//go:build !darwin && !windows

package audiolevel

import "context"

func captureSystemLevel(context.Context, func(*[fftSize]float32, int)) {}
