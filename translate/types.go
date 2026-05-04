package translate

import (
	"context"
	"errors"
	"strings"
)

type Translate interface {
	Translate(ctx context.Context, r *Request) (*Response, error)
	Search(ctx context.Context, r *Request) (*Response, error) //seraches for existing translation
	Close()
	SupportedLanguage() []Language
	IsLanguageSupported(from Language, to Language) bool
	SupportedModels() []string
}

type Language string

var (
	LanguageEnglish  Language = "en"
	LanguageJapanese Language = "jpn"
)

type Request struct {
	Image                          []byte
	Text                           string
	ToLanguage                     Language
	SurroundingContext             string
	PersistTranslationData         bool
	ShareTranslationWithOtherUsers bool

	Speaker       string
	GameTitle     string
	Scene         string
	Glossary      map[string]string
	PreferLiteral bool
}

type Response struct {
	Request *Request
	Text    string
}

var (
	ErrNotFound            = errors.New("translation not found")
	ErrEmptyText           = errors.New("empty translation text")
	ErrUnsupportedLanguage = errors.New("unsupported language")
	ErrImageUnsupported    = errors.New("image translation unsupported by this client")
)

func cleanText(s string) string {
	return strings.TrimSpace(s)
}

func validateRequest(r *Request) error {
	if r == nil {
		return errors.New("nil request")
	}
	if cleanText(r.Text) == "" && len(r.Image) == 0 {
		return ErrEmptyText
	}
	return nil
}

func defaultHTTPTimeoutSeconds() int {
	return 60
}
