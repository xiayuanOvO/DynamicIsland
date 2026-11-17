package audiolevel

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

func captureSystemLevel(ctx context.Context, report func(*[fftSize]float32, int)) {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return
	}

	cmd := exec.CommandContext(ctx, filepath.Join(resources, "audio-level"))
	input, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	defer output.Close()
	cmd.Cancel = input.Close
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return
	}
	defer func() {
		input.Close()
		output.Close()
		cmd.Wait()
	}()

	var header [4]byte
	if _, err := io.ReadFull(output, header[:]); err != nil {
		return
	}
	rate := int(binary.LittleEndian.Uint32(header[:]))
	if rate < 8000 || rate > 384000 {
		return
	}
	var packet [fftSize * 4]byte
	var samples [fftSize]float32
	for {
		if _, err := io.ReadFull(output, packet[:]); err != nil {
			return
		}
		for i := range samples {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(packet[i*4:]))
		}
		report(&samples, rate)
	}
}
