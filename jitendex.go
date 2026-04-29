package jpndict

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/Seann-Moser/jpndict/audio"
	"github.com/Seann-Moser/jpndict/mdict"
	"golang.org/x/net/html"
)

const defaultZip = "https://github.com/stephenmk/stephenmk.github.io/releases/latest/download/jitendex-mdict.zip"

var _ Dictonary = &JiTenDex{}

type JiTenDex struct {
	zipFilePath  string
	Dir          string
	AudioEnabled bool
	m            *mdict.MDX
	a            audio.AudioProvider
}

func NewJiTenDex(dir string, zipFilePath string, audioEnabled bool) (Dictonary, error) {
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(cache, "jpndict")
	}
	if zipFilePath == "" {
		zipFilePath = defaultZip
	}
	var a audio.AudioProvider
	var err error
	if audioEnabled {
		a, err = audio.NewLanguagePod101(filepath.Join(dir, "audio"))
		if err != nil {
			return nil, err
		}
	}
	return &JiTenDex{
		Dir:          dir,
		zipFilePath:  zipFilePath,
		a:            a,
		AudioEnabled: audioEnabled,
	}, nil

}

func (j *JiTenDex) Download() error {
	mdx := filepath.Join(j.Dir, "jitendex", "jitendex.mdx")
	if _, err := os.Stat(mdx); os.IsNotExist(err) {
		err := DownloadAndUnzip(context.Background(),
			j.zipFilePath,
			j.Dir,
		)
		if err != nil {
			return err
		}
	}

	m, err := mdict.OpenMDX(mdx)
	if err != nil {
		return err
	}
	j.m = m
	return err
}
func (j *JiTenDex) lookupFollow(key string, maxDepth int) ([]byte, string, error) {
	current := key

	for i := 0; i < maxDepth; i++ {
		data, err := j.m.Lookup(current)
		if err != nil {
			return nil, "", err
		}

		s := strings.TrimSpace(strings.TrimRight(string(data), "\x00"))

		if strings.HasPrefix(s, "@@@LINK=") {
			current = strings.TrimSpace(strings.TrimPrefix(s, "@@@LINK="))
			continue
		}

		return data, current, nil
	}

	return nil, "", fmt.Errorf("too many redirects for %s", key)
}

func (j *JiTenDex) Search(search string) (*Response, error) {
	data, key, err := j.lookupFollow(search, 5)
	if err != nil {
		return nil, err
	}

	htmlText := strings.TrimRight(string(data), "\x00")

	resp := &Response{
		Query: search,
		Key:   key,
		HTML:  htmlText,
		Text:  StripHTML(htmlText),
		Raw:   data,
	}

	entry, err := ParseJitendexHTML(htmlText)
	if err == nil {
		resp.Entry = entry
		if j.AudioEnabled && resp.Entry.Pronunciation != nil {
			aud, err := j.a.GetAudio(context.Background(), resp.Entry.Headword, resp.Entry.Reading)
			if err == nil {
				resp.Entry.Pronunciation.Audio = aud.Path
			} else {
				slog.Error("unable to find audio for word", "word", search)
			}
		}
	}

	return resp, nil
}
func StripHTML(s string) string {
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}

	var b strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			txt := strings.TrimSpace(n.Data)
			if txt != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(txt)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)
	return strings.TrimSpace(b.String())
}

func ParseJitendexHTML(raw string) (*Entry, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return nil, err
	}

	entry := &Entry{}

	headline := doc.Find(".headline").First()
	entry.Headword = cleanText(headline.Find(".headword ruby").First().Contents().FilterFunction(func(i int, s *goquery.Selection) bool {
		return goquery.NodeName(s) != "rt"
	}).Text())

	entry.Reading = cleanText(headline.Find(".headword rt").First().Text())
	entry.IsPriority = headline.HasClass("priority") || headline.Find(".priority-symbol").Length() > 0

	if p := parsePronunciation(doc); p != nil {
		entry.Pronunciation = p
	}

	doc.Find(".sense-group").Each(func(_ int, sg *goquery.Selection) {
		sense := Sense{}

		sense.Number, _ = sg.Find(".sense").First().Attr("data-sense-number")

		sg.Find(".part-of-speech-info").Each(func(_ int, pos *goquery.Selection) {
			t := cleanText(pos.Text())
			if t != "" {
				sense.PartsOfSpeech = append(sense.PartsOfSpeech, t)
			}
		})

		sg.Find(".glossary .gloss").Each(func(_ int, gloss *goquery.Selection) {
			t := cleanText(gloss.Text())
			if t != "" {
				sense.Glosses = append(sense.Glosses, t)
			}
		})

		sg.Find(".sense-note-content").Each(func(_ int, note *goquery.Selection) {
			t := cleanText(note.Text())
			if t != "" {
				sense.Notes = append(sense.Notes, t)
			}
		})

		sg.Find(".ex-sent").Each(func(_ int, ex *goquery.Selection) {
			e := Example{
				Japanese: cleanText(ex.Find(".ex-sent-ja-content").Text()),
				English:  cleanText(ex.Find(".ex-sent-en-content").Text()),
			}
			if e.Japanese != "" || e.English != "" {
				sense.Examples = append(sense.Examples, e)
			}
		})

		sg.Find(".xref").Each(func(_ int, xr *goquery.Selection) {
			a := xr.Find("a").First()

			href, _ := a.Attr("href")
			targetID, _ := a.Attr("data-target-id")

			x := XRef{
				Word: cleanText(a.Find("ruby").First().Contents().FilterFunction(func(i int, s *goquery.Selection) bool {
					return goquery.NodeName(s) != "rt"
				}).Text()),
				Reading:  cleanText(a.Find("rt").First().Text()),
				Glossary: cleanText(xr.Find(".xref-glossary").Text()),
				Href:     href,
				TargetID: targetID,
			}

			if x.Word != "" || x.Href != "" {
				sense.XRefs = append(sense.XRefs, x)
			}
		})

		if len(sense.Glosses) > 0 || len(sense.PartsOfSpeech) > 0 {
			entry.Senses = append(entry.Senses, sense)
		}
	})

	doc.Find(".entry-footnotes a").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		entry.Links = append(entry.Links, ReferenceLink{
			Label: cleanText(a.Text()),
			Href:  href,
		})
	})

	return entry, nil
}

func parsePronunciation(doc *goquery.Document) *Pronunciation {
	audio := doc.Find(".audio").First()
	if audio.Length() == 0 {
		return nil
	}

	href, _ := audio.Find("a").First().Attr("href")

	p := &Pronunciation{
		Text:  cleanText(audio.Find(".pronunciation-text").Text()),
		Audio: href,
	}

	if title, ok := audio.Find("img").First().Attr("title"); ok {
		p.Pitch = title
	}

	return p
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
