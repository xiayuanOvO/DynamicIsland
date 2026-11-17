package audiolevel

import (
	"math"
	"testing"
	"time"
)

func TestSpectrumSeparatesTones(t *testing.T) {
	const rate = 48000
	var analyzer spectrum
	var samples [fftSize]float32
	for i := range samples {
		t := float64(i) / rate
		samples[i] = float32(0.4*math.Sin(2*math.Pi*120*t) + 0.4*math.Sin(2*math.Pi*4800*t))
	}
	bands := analyzer.measure(&samples, rate)
	if maxBand(bands[:14]) < 0.45 || maxBand(bands[35:]) < 0.45 {
		t.Fatalf("tones did not reach their bands: %v", bands)
	}
	if maxBand(bands[18:32]) > 0.08 {
		t.Fatalf("unrelated bands responded to both tones: %v", bands[18:32])
	}
	var silence [fftSize]float32
	if analyzer.measure(&silence, rate) != (Bars{}) {
		t.Fatal("silence produced a nonzero spectrum")
	}
}

func TestBarsReactInPlace(t *testing.T) {
	var bars, levels Bars
	levels[3], levels[45] = 0.9, 0.4
	bars.Update(levels, 16*time.Millisecond)
	bars.Update(Bars{}, 16*time.Millisecond)
	if bars[3] <= bars[45] || bars[3] >= 0.9 || bars[45] <= 0 || bars[4] != 0 {
		t.Fatalf("bands did not ease independently in place: %v", bars)
	}
	for range 48 {
		bars.Update(Bars{}, 16*time.Millisecond)
	}
	if bars != (Bars{}) {
		t.Fatal("bars retained visible levels during silence")
	}
}

func TestBarAnimationTiming(t *testing.T) {
	var target, rising, falling Bars
	target[10], falling[10] = 1, 1
	rising.Update(target, 16*time.Millisecond)
	falling.Update(Bars{}, 16*time.Millisecond)
	if rising[10] <= 1-falling[10] || rising[10] >= 1 {
		t.Fatalf("expected a smooth, faster attack than release: up=%v down=%v", rising[10], 1-falling[10])
	}

	var combined Bars
	rising.Update(target, 16*time.Millisecond)
	combined.Update(target, 32*time.Millisecond)
	if math.Abs(float64(rising[10]-combined[10])) > 1e-6 {
		t.Fatalf("height changed with frame rate: %v vs %v", rising[10], combined[10])
	}
}

func maxBand(bands []float32) float32 {
	var highest float32
	for _, band := range bands {
		highest = max(highest, band)
	}
	return highest
}

func TestBarsIgnoreSmallFluctuations(t *testing.T) {
	const frame = 16 * time.Millisecond
	bars := Bars{12: 0.5}
	for _, level := range []float32{0.52, 0.49, 0.53, 0.48} {
		bars.Update(Bars{12: level}, frame)
		if bars[12] != 0.5 {
			t.Fatalf("small fluctuation to %v moved bar to %v", level, bars[12])
		}
	}
	bars.Update(Bars{12: 0.8}, frame)
	if bars[12] <= 0.6 {
		t.Fatalf("clear beat was damped too far: %v", bars[12])
	}
}
