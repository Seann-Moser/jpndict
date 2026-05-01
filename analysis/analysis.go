package analysis

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

type Analysis struct {
	Sentence    string
	Tokens      []Token
	Particles   []Token
	Verbs       []Token
	Auxiliaries []Token
}

type Token struct {
	Surface       string
	BaseForm      string
	Reading       string
	Pronunciation string
	POS           []string
	InflectType   string
	InflectForm   string
}

var (
	tokenizerOnce sync.Once
	sharedToken   *tokenizer.Tokenizer
	sharedErr     error
)

func AnalyzeSentence(sentence string) (Analysis, error) {
	sentence = strings.TrimSpace(sentence)
	if sentence == "" {
		return Analysis{}, fmt.Errorf("sentence analysis requires text")
	}

	t, err := sharedTokenizer()
	if err != nil {
		return Analysis{}, err
	}

	out := Analysis{Sentence: sentence}
	for _, raw := range t.Tokenize(sentence) {
		token := newToken(raw)
		if token.Surface == "" {
			continue
		}
		out.Tokens = append(out.Tokens, token)
		switch token.POSMajor() {
		case "助詞":
			out.Particles = append(out.Particles, token)
		case "動詞":
			out.Verbs = append(out.Verbs, token)
		case "助動詞":
			out.Auxiliaries = append(out.Auxiliaries, token)
		}
	}
	return out, nil
}

func ExtractSentence(text, needle string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	needle = strings.TrimSpace(needle)
	segments := splitSentences(text)
	if len(segments) == 0 {
		return text
	}
	if needle == "" {
		return segments[0]
	}
	for _, segment := range segments {
		if strings.Contains(segment, needle) {
			return segment
		}
	}
	return segments[0]
}

func (t Token) POSMajor() string {
	if len(t.POS) == 0 {
		return ""
	}
	return cleanFeature(t.POS[0])
}

func (t Token) POSLabel() string {
	parts := make([]string, 0, len(t.POS))
	for _, part := range t.POS {
		part = cleanFeature(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " / ")
}

func (t Token) InflectionLabel() string {
	parts := make([]string, 0, 2)
	if part := cleanFeature(t.InflectType); part != "" {
		parts = append(parts, part)
	}
	if part := cleanFeature(t.InflectForm); part != "" {
		parts = append(parts, part)
	}
	return strings.Join(parts, " / ")
}

func (t Token) CompactLabel() string {
	label := strings.TrimSpace(t.Surface)
	if label == "" {
		return ""
	}
	if pos := t.POSLabel(); pos != "" {
		return label + " (" + pos + ")"
	}
	return label
}

func (t Token) VerbLabel() string {
	label := strings.TrimSpace(t.Surface)
	if label == "" {
		return ""
	}
	base := cleanFeature(t.BaseForm)
	if base != "" && base != label {
		label += " -> " + base
	}
	if inflection := t.InflectionLabel(); inflection != "" {
		label += " (" + inflection + ")"
	}
	return label
}

func (t Token) StructureLabel() string {
	label := strings.TrimSpace(t.Surface)
	if label == "" {
		return ""
	}
	if pos := t.POSLabel(); pos != "" {
		label += " [" + pos + "]"
	}
	base := cleanFeature(t.BaseForm)
	if base != "" && base != label {
		label += " -> " + base
	}
	return label
}

func sharedTokenizer() (*tokenizer.Tokenizer, error) {
	tokenizerOnce.Do(func() {
		sharedToken, sharedErr = tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	})
	return sharedToken, sharedErr
}

func newToken(raw tokenizer.Token) Token {
	token := Token{
		Surface: strings.TrimSpace(raw.Surface),
		POS:     raw.POS(),
	}
	if v, ok := raw.BaseForm(); ok {
		token.BaseForm = cleanFeature(v)
	}
	if v, ok := raw.Reading(); ok {
		token.Reading = cleanFeature(v)
	}
	if v, ok := raw.Pronunciation(); ok {
		token.Pronunciation = cleanFeature(v)
	}
	if v, ok := raw.InflectionalType(); ok {
		token.InflectType = cleanFeature(v)
	}
	if v, ok := raw.InflectionalForm(); ok {
		token.InflectForm = cleanFeature(v)
	}
	return token
}

func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := make([]string, 0, 4)
	var b strings.Builder
	for _, r := range text {
		b.WriteRune(r)
		if isSentenceBoundary(r) {
			if part := strings.TrimSpace(b.String()); part != "" {
				parts = append(parts, part)
			}
			b.Reset()
		}
	}
	if part := strings.TrimSpace(b.String()); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '\n', '。', '！', '？', '!', '?':
		return true
	default:
		return false
	}
}

func cleanFeature(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "*" {
		return ""
	}
	return value
}
