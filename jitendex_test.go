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
