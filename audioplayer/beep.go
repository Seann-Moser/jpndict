package audioplayer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
)

type BeepPlayer struct {
	mu sync.Mutex

	file     *os.File
	streamer beep.StreamSeekCloser
	format   beep.Format

	ctrl   *beep.Ctrl
	volume *effects.Volume

	state  State
	path   string
	closed bool

	speakerReady bool
	muted        bool
	lastVolume   float64
}

func NewBeepPlayer() (*BeepPlayer, error) {
	return &BeepPlayer{
		state:      StateIdle,
		lastVolume: 1.0,
	}, nil
}

func (p *BeepPlayer) Open(ctx context.Context, path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}

	if p.streamer != nil {
		_ = p.streamer.Close()
		p.streamer = nil
	}
	if p.file != nil {
		_ = p.file.Close()
		p.file = nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	f, err := os.Open(abs)
	if err != nil {
		return err
	}

	streamer, format, err := decodeByExt(abs, f)
	if err != nil {
		_ = f.Close()
		return err
	}

	if !p.speakerReady {
		if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
			_ = streamer.Close()
			_ = f.Close()
			return fmt.Errorf("init speaker: %w", err)
		}
		p.speakerReady = true
	}

	p.file = f
	p.streamer = streamer
	p.format = format
	p.path = abs
	p.state = StateReady

	p.ctrl = &beep.Ctrl{
		Streamer: p.streamer,
		Paused:   true,
	}

	p.volume = &effects.Volume{
		Streamer: p.ctrl,
		Base:     2,
		Volume:   volumeToBeep(p.lastVolume),
		Silent:   p.muted,
	}

	speaker.Play(p.volume)

	return nil
}

func (p *BeepPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if p.streamer == nil || p.ctrl == nil {
		return ErrNoFileOpen
	}

	speaker.Lock()
	p.ctrl.Paused = false
	speaker.Unlock()

	p.state = StatePlaying
	return nil
}

func (p *BeepPlayer) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if p.streamer == nil || p.ctrl == nil {
		return ErrNoFileOpen
	}

	speaker.Lock()
	p.ctrl.Paused = true
	speaker.Unlock()

	p.state = StatePaused
	return nil
}

func (p *BeepPlayer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if p.streamer == nil {
		return nil
	}

	speaker.Lock()
	if p.ctrl != nil {
		p.ctrl.Paused = true
	}
	_ = p.streamer.Seek(0)
	speaker.Unlock()

	p.state = StateStopped
	return nil
}

func (p *BeepPlayer) Seek(pos time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}
	if p.streamer == nil {
		return ErrNoFileOpen
	}

	samples := p.format.SampleRate.N(pos)

	speaker.Lock()
	err := p.streamer.Seek(samples)
	speaker.Unlock()

	if err != nil {
		return fmt.Errorf("beep seek: %w", err)
	}

	return nil
}

func (p *BeepPlayer) Position() (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, ErrClosed
	}
	if p.streamer == nil {
		return 0, ErrNoFileOpen
	}

	speaker.Lock()
	pos := p.streamer.Position()
	speaker.Unlock()

	return p.format.SampleRate.D(pos), nil
}

func (p *BeepPlayer) Duration() (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, ErrClosed
	}
	if p.streamer == nil {
		return 0, ErrNoFileOpen
	}

	return p.format.SampleRate.D(p.streamer.Len()), nil
}

func (p *BeepPlayer) SetVolume(volume float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}

	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}

	p.lastVolume = volume

	if p.volume != nil {
		speaker.Lock()
		p.volume.Volume = volumeToBeep(volume)
		p.volume.Silent = p.muted || volume == 0
		speaker.Unlock()
	}

	return nil
}

func (p *BeepPlayer) Volume() (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, ErrClosed
	}

	return p.lastVolume, nil
}

func (p *BeepPlayer) SetMuted(muted bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrClosed
	}

	p.muted = muted

	if p.volume != nil {
		speaker.Lock()
		p.volume.Silent = muted || p.lastVolume == 0
		speaker.Unlock()
	}

	return nil
}

func (p *BeepPlayer) Muted() (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return false, ErrClosed
	}

	return p.muted, nil
}

func (p *BeepPlayer) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *BeepPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	if p.ctrl != nil {
		speaker.Lock()
		p.ctrl.Paused = true
		speaker.Unlock()
	}

	if p.streamer != nil {
		_ = p.streamer.Close()
		p.streamer = nil
	}

	if p.file != nil {
		_ = p.file.Close()
		p.file = nil
	}

	p.closed = true
	p.state = StateClosed
	return nil
}

func decodeByExt(path string, f *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".mp3":
		return mp3.Decode(f)
	case ".wav":
		return wav.Decode(f)
	case ".ogg":
		return vorbis.Decode(f)
	case ".flac":
		return flac.Decode(f)
	default:
		return nil, beep.Format{}, fmt.Errorf("unsupported audio file extension %q", ext)
	}
}

// effects.Volume uses logarithmic-ish volume.
// 0.0 should be silent, 1.0 should be neutral.
func volumeToBeep(v float64) float64 {
	if v <= 0 {
		return -8
	}
	if v >= 1 {
		return 0
	}

	// Simple mapping:
	// 1.0 => 0
	// 0.5 => -1
	// 0.25 => -2
	// etc.
	x := 0.0
	for v < 1 {
		v *= 2
		x--
		if x <= -8 {
			return -8
		}
	}
	return x
}
