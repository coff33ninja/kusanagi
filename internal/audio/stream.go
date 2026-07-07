//go:build windows

package audio

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

func StartStream(sampleRate int, chunkDuration time.Duration) (<-chan []byte, func() error, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(msg string) {
		slog.Debug("malgo: audio", "message", msg)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("malgo InitContext: %w", err)
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = uint32(sampleRate)

	out := make(chan []byte, 32)
	var mu sync.Mutex
	closed := false

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: func(pOutputSample, pInputSamples []byte, framecount uint32) {
			mu.Lock()
			if closed {
				mu.Unlock()
				return
			}
			mu.Unlock()
			if len(pInputSamples) > 0 {
				chunk := make([]byte, len(pInputSamples))
				copy(chunk, pInputSamples)
				select {
				case out <- chunk:
				default:
				}
			}
		},
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return nil, nil, fmt.Errorf("malgo InitDevice: %w", err)
	}

	err = device.Start()
	if err != nil {
		device.Uninit()
		ctx.Uninit()
		ctx.Free()
		return nil, nil, fmt.Errorf("malgo device Start: %w", err)
	}

	closeFn := func() error {
		mu.Lock()
		if closed {
			mu.Unlock()
			return nil
		}
		closed = true
		mu.Unlock()

		device.Uninit()
		ctx.Uninit()
		ctx.Free()
		close(out)
		return nil
	}

	return out, closeFn, nil
}
