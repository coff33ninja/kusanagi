//go:build windows

package audio

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/gen2brain/malgo"
)

type Player struct {
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	sampleRate int
	mu         sync.Mutex
	buffer     []byte
	pos        int
}

func NewPlayer(sampleRate int) (*Player, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("malgo InitContext: %w", err)
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 1
	deviceConfig.SampleRate = uint32(sampleRate)

	p := &Player{ctx: ctx, sampleRate: sampleRate}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: func(pOutputSample, pInputSamples []byte, framecount uint32) {
			p.mu.Lock()
			if p.pos >= len(p.buffer) {
				p.buffer = p.buffer[:0]
				p.pos = 0
				for i := range pOutputSample {
					pOutputSample[i] = 0
				}
				p.mu.Unlock()
				return
			}
			n := copy(pOutputSample, p.buffer[p.pos:])
			p.pos += n
			for i := n; i < len(pOutputSample); i++ {
				pOutputSample[i] = 0
			}
			p.mu.Unlock()
		},
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("malgo InitDevice: %w", err)
	}
	p.device = device

	if err := device.Start(); err != nil {
		device.Uninit()
		ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("malgo device Start: %w", err)
	}

	return p, nil
}

func (p *Player) Play(audioData []byte) error {
	p.mu.Lock()
	p.buffer = append(p.buffer, audioData...)
	p.mu.Unlock()
	return nil
}

func (p *Player) PlayStream(ch <-chan []byte) error {
	for chunk := range ch {
		if err := p.Play(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (p *Player) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.device != nil {
		p.device.Uninit()
		p.device = nil
	}
	if p.ctx != nil {
		p.ctx.Uninit()
		p.ctx.Free()
		p.ctx = nil
	}
	slog.Debug("playback: player closed")
	return nil
}
