package audioplayer

import "errors"

type State string

const (
	StateIdle    State = "idle"
	StateReady   State = "ready"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
	StateClosed  State = "closed"
)

var (
	ErrUnsupportedBackend = errors.New("unsupported audio backend")
	ErrNoFileOpen         = errors.New("no audio file is open")
	ErrClosed             = errors.New("audio player is closed")
)
