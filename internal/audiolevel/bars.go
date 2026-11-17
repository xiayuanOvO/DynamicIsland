package audiolevel

import (
	"math"
	"math/bits"
	"math/cmplx"
	"time"
)

const fftSize = 4096

type Bars [52]float32

func (b *Bars) Update(levels Bars, elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}
	attack := float32(-math.Expm1(-float64(elapsed) / float64(24*time.Millisecond)))
	release := float32(-math.Expm1(-float64(elapsed) / float64(105*time.Millisecond)))
	const deadband = 0.045 // Less than one pixel across the visible height range.
	for i, level := range levels {
		delta := level - b[i]
		if level > 0 && delta > -deadband && delta < deadband {
			continue
		}
		amount := release
		if delta > 0 {
			amount = attack
		}
		height := b[i] + delta*amount
		if level == 0 && height < 0.002 {
			height = 0
		}
		b[i] = height
	}
}

var hannWindow = func() [fftSize]float64 {
	var window [fftSize]float64
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/(fftSize-1))
	}
	return window
}()

var twiddles = func() [fftSize / 2]complex128 {
	var roots [fftSize / 2]complex128
	for i := range roots {
		roots[i] = cmplx.Rect(1, -2*math.Pi*float64(i)/fftSize)
	}
	return roots
}()

type spectrum struct {
	frequency [fftSize]complex128
}

func (s *spectrum) measure(samples *[fftSize]float32, rate int) Bars {
	for i, sample := range samples {
		value := float64(sample)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			value = 0
		}
		index := bits.Reverse16(uint16(i)) >> 4
		s.frequency[index] = complex(value*hannWindow[i], 0)
	}
	for width := 2; width <= fftSize; width *= 2 {
		half := width / 2
		for start := 0; start < fftSize; start += width {
			for offset := 0; offset < half; offset++ {
				left, right := start+offset, start+offset+half
				a := s.frequency[left]
				b := s.frequency[right] * twiddles[offset*fftSize/width]
				s.frequency[left], s.frequency[right] = a+b, a-b
			}
		}
	}

	var bars Bars
	const low = 55.0
	high := min(16000, float64(rate)*0.48)
	for i := range bars {
		first := low * math.Pow(high/low, float64(i)/float64(len(bars)))
		last := low * math.Pow(high/low, float64(i+1)/float64(len(bars)))
		scale := fftSize / float64(rate)
		from, to := int(math.Round(first*scale)), int(math.Round(last*scale))
		var peak float64
		for bin := max(1, from); bin <= to; bin++ {
			peak = max(peak, cmplx.Abs(s.frequency[bin]))
		}
		// Hann-window normalization keeps the result comparable across bins.
		amplitude := peak * 4 / fftSize
		bars[i] = float32(min(1, math.Log1p(45*amplitude)/math.Log(26)))
	}
	return bars
}
