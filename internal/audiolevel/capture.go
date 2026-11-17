package audiolevel

import (
	"context"
	"sync"
	"time"
)

const SampleInterval = 32 * time.Millisecond

type Capture struct {
	cancel  context.CancelFunc
	mu      sync.Mutex
	levels  Bars
	stopped bool
}

func Start() *Capture {
	ctx, cancel := context.WithCancel(context.Background())
	capture := &Capture{cancel: cancel}
	go func() {
		var analyzer spectrum
		captureSystemLevel(ctx, func(samples *[fftSize]float32, rate int) {
			levels := analyzer.measure(samples, rate)
			capture.mu.Lock()
			capture.levels = levels
			capture.mu.Unlock()
		})
		capture.mu.Lock()
		capture.levels, capture.stopped = Bars{}, true
		capture.mu.Unlock()
	}()
	return capture
}

func (c *Capture) Levels() (Bars, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.levels, !c.stopped
}

func (c *Capture) Stop() { c.cancel() }
