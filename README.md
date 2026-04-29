# jpn-dict

`jpn-dict` provides a Go interface for looking up Japanese dictionary entries from JiTenDex.

## JiTenDex Usage

`JiTenDex` downloads the MDict archive, opens the local `jitendex.mdx` file, and returns both raw HTML/text and a parsed entry structure.

### Install

```bash
go get github.com/Seann-Moser/jpn-dict
```

### Basic Example

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	jpndict "github.com/Seann-Moser/jpn-dict"
)

func main() {
	dict, err := jpndict.NewJiTenDex("", "", true)
	if err != nil {
		log.Fatal(err)
	}

	if err := dict.Download(); err != nil {
		log.Fatal(err)
	}

	resp, err := dict.Search("かぜ")
	if err != nil {
		log.Fatal(err)
	}

	out, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(out))
}
```

### Constructor

```go
dict, err := jpndict.NewJiTenDex(dir, zipFilePath, audioEnabled)
```

Arguments:

- `dir`: local cache directory. If empty, the package uses `os.UserCacheDir()/jpndict`.
- `zipFilePath`: path or URL to the JiTenDex MDict zip. If empty, it uses the latest release URL baked into the package.
- `audioEnabled`: when `true`, the dictionary tries to fetch pronunciation audio and stores it under `<dir>/audio`.

### Required Flow

Call `Download()` before `Search()`.

`Download()`:

- downloads and extracts the JiTenDex MDict assets when they are not already present
- opens `<dir>/jitendex/jitendex.mdx`

`Search(query)`:

- resolves MDict redirect entries such as `@@@LINK=...`
- returns the original HTML
- returns stripped plain text
- parses structured fields like headword, reading, pronunciation, glosses, examples, and cross references

### Response Shape

`Search()` returns `*jpn_dict.Response`:

```go
type Response struct {
	Query string
	Key   string
	HTML  string
	Text  string
	Raw   []byte
	Entry *Entry
}
```

Useful parsed fields in `Entry` include:

- `Headword`
- `Reading`
- `IsPriority`
- `Pronunciation`
- `Senses`
- `Links`

Each `Sense` may contain:

- `PartsOfSpeech`
- `Glosses`
- `Notes`
- `Examples`
- `XRefs`

### Local Files

After `Download()`, JiTenDex data is stored under:

```text
<dir>/jitendex/jitendex.mdx
<dir>/jitendex/jitendex.mdd
<dir>/jitendex/jitendex.css
<dir>/jitendex/common.css
```

If audio is enabled, downloaded audio is cached under:

```text
<dir>/audio
```

### Notes

- The current public interface returned by `NewJiTenDex` is `Dictonary`.
- If you need JiTenDex-specific configuration, set it when constructing the dictionary.
- The package name is `jpn_dict`, so importing with an alias like `jpndict` makes examples cleaner.
