package jpndict

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewJiTenDex(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	j, _ := NewJiTenDex(filepath.Join(wd, "tmp"), "", true)
	err = j.Download()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := j.Search("かぜ")
	if err != nil {
		t.Fatal(err)
	}
	d, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	print(string(d))
}

type tc struct {
	search        string
	expectedKana  string
	expectedKanji string
	hasAudio      bool
}

func TestJiTenDex_Search(t *testing.T) {
	tcs := []tc{
		{
			search:       "最大",
			expectedKana: "さいだい",
			hasAudio:     true,
		},
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	j, _ := NewJiTenDex(filepath.Join(wd, "tmp"), "", true)
	err = j.Download()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tcs {
		t.Run(tc.search, func(t *testing.T) {
			resp, err := j.Search(tc.search)
			if err != nil {
				t.Errorf("failed to find word")
				return
			}
			if resp.Entry == nil {
				t.Errorf("expected entry")
				return
			}
			if resp.Entry.Reading != tc.expectedKana {
				t.Errorf("reading missmatch expected:%s got:%s", tc.expectedKana, resp.Entry.Reading)
			}
			if tc.hasAudio && (resp.Entry.Pronunciation == nil || resp.Entry.Pronunciation.Audio == "") {
				t.Errorf("expected audio for %s", tc.search)
			}
		})
	}
}
