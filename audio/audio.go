package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
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

type LanguagePod101 struct {
	Client           *http.Client
	CacheDir         string
	NegativeCacheTTL time.Duration
}

func NewLanguagePod101(cacheDir string) (*LanguagePod101, error) {
	if cacheDir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		cacheDir = filepath.Join(cache, "jpndict", "audio")
	}

	return &LanguagePod101{
		CacheDir:         cacheDir,
		NegativeCacheTTL: 7 * 24 * time.Hour, // tweak as needed
		Client: &http.Client{
			Timeout: 20 * time.Second,

			// Important: capture redirect instead of automatically following it.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}
func (p *LanguagePod101) negativePath(kanji, kana string) string {
	return filepath.Join(p.CacheDir, "negative", cacheName(kanji, kana)+".miss")
}

func (p *LanguagePod101) isNegativeCached(kanji, kana string) bool {
	path := p.negativePath(kanji, kana)

	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if p.NegativeCacheTTL > 0 && time.Since(info.ModTime()) > p.NegativeCacheTTL {
		_ = os.Remove(path)
		return false
	}

	return true
}

func (p *LanguagePod101) writeNegativeCache(kanji, kana string) {
	path := p.negativePath(kanji, kana)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte("miss"), 0644)
}

func (p *LanguagePod101) clearNegativeCache(kanji, kana string) {
	_ = os.Remove(p.negativePath(kanji, kana))
}

func (p *LanguagePod101) GetAudio(ctx context.Context, kanji, kana string) (*AudioFile, error) {
	if p.Client == nil {
		p.Client = http.DefaultClient
	}

	if err := os.MkdirAll(p.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create audio cache dir: %w", err)
	}
	sourceURL := p.buildURL(kanji, kana)
	cachePath := filepath.Join(p.CacheDir, cacheName(kanji, kana)+".mp3")

	if _, err := os.Stat(cachePath); err == nil {
		p.clearNegativeCache(kanji, kana)
		return &AudioFile{
			Kanji:     kanji,
			Kana:      kana,
			Path:      cachePath,
			CacheHit:  true,
			SourceURL: sourceURL,
		}, nil
	}
	// Only use the negative cache when we don't already have a real file.
	if p.isNegativeCached(kanji, kana) {
		return nil, fmt.Errorf("audio not found (cached)")
	}

	redirectURL, err := p.findRedirect(ctx, sourceURL)
	if err != nil {
		if errors.Is(err, ErrAudioNotFound) {
			p.writeNegativeCache(kanji, kana)
		}
		return nil, err
	}

	if err := downloadFile(ctx, p.Client, redirectURL, cachePath); err != nil {
		_ = os.Remove(cachePath)
		return nil, err
	}
	p.clearNegativeCache(kanji, kana)

	return &AudioFile{
		Kanji:     kanji,
		Kana:      kana,
		Path:      cachePath,
		CacheHit:  false,
		SourceURL: sourceURL,
		FinalURL:  redirectURL,
	}, nil
}

func (p *LanguagePod101) buildURL(kanji, kana string) string {
	u := url.URL{
		Scheme: "https",
		Host:   "assets.languagepod101.com",
		Path:   "/dictionary/japanese/audiomp3.php",
	}

	q := u.Query()
	q.Set("kanji", kanji)
	q.Set("kana", kana)
	u.RawQuery = q.Encode()

	return u.String()
}

var ErrAudioNotFound = errors.New("audio not found")

func (p *LanguagePod101) findRedirect(ctx context.Context, sourceURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", fmt.Errorf("create redirect request: %w", err)
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request audio redirect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("%w: expected redirect, got status %s", ErrAudioNotFound, resp.Status)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("%w: redirect missing Location header", ErrAudioNotFound)
	}

	redirectURL, err := resp.Request.URL.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse redirect location: %w", err)
	}

	return redirectURL.String(), nil
}

func downloadFile(ctx context.Context, client *http.Client, fileURL, dest string) error {
	// This client should follow redirects normally for final asset download.
	dlClient := *client
	dlClient.CheckRedirect = nil

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}

	resp, err := dlClient.Do(req)
	if err != nil {
		return fmt.Errorf("download audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download audio: bad status %s", resp.Status)
	}

	tmp := dest + ".tmp"

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create temp audio file: %w", err)
	}

	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()

	if copyErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write audio file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close audio file: %w", closeErr)
	}

	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit audio file: %w", err)
	}

	return nil
}

func cacheName(kanji, kana string) string {
	sum := sha256.Sum256([]byte(kanji + "\x00" + kana))
	return hex.EncodeToString(sum[:])
}
