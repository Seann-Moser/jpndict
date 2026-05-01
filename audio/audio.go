package audio

import (
	"context"
)

type AudioProvider interface {
	GetAudio(ctx context.Context, kanji, kana string) (*AudioFile, error)
}

type AudioFile struct {
	Kanji     string
	Kana      string
	Path      string
	CacheHit  bool
	SourceURL string
	FinalURL  string
}
