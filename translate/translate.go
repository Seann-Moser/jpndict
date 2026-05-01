package translate

import "context"

type Translate interface {
	Translate(ctx context.Context, r *Request) (*Response, error)
	Search(ctx context.Context, r *Request) (*Response, error) //seraches for existing translation
	Load()
	Close()
}

type Request struct {
	Image                          []byte
	Text                           string
	ToLanguage                     string
	SurroundingContext             string
	PersistTranslationData         bool
	ShareTranslationWithOtherUsers bool
}

type Response struct {
	Request *Request
	Text    string
}
