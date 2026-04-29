package jpndict

type Dictonary interface {
	Download() error
	Search(data string) (*Response, error)
}

type Response struct {
	Query string `json:"query"`
	Key   string `json:"key"`

	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`
	Raw  []byte `json:"raw,omitempty"`

	Entry *Entry `json:"entry,omitempty"`
}

type Entry struct {
	Headword      string          `json:"headword,omitempty"`
	Reading       string          `json:"reading,omitempty"`
	IsPriority    bool            `json:"isPriority,omitempty"`
	Pronunciation *Pronunciation  `json:"pronunciation,omitempty"`
	Senses        []Sense         `json:"senses,omitempty"`
	Links         []ReferenceLink `json:"links,omitempty"`
}

type Pronunciation struct {
	Text      string `json:"text,omitempty"`
	Audio     string `json:"audio,omitempty"`
	Pitch     string `json:"pitch,omitempty"`
	LocalPath string `json:"localPath,omitempty"`
}

type Sense struct {
	Number        string    `json:"number,omitempty"`
	PartsOfSpeech []string  `json:"partsOfSpeech,omitempty"`
	Glosses       []string  `json:"glosses,omitempty"`
	Notes         []string  `json:"notes,omitempty"`
	Examples      []Example `json:"examples,omitempty"`
	XRefs         []XRef    `json:"xrefs,omitempty"`
}

type Example struct {
	Japanese string `json:"japanese,omitempty"`
	English  string `json:"english,omitempty"`
}

type XRef struct {
	Word     string `json:"word,omitempty"`
	Reading  string `json:"reading,omitempty"`
	Glossary string `json:"glossary,omitempty"`
	Href     string `json:"href,omitempty"`
	TargetID string `json:"targetId,omitempty"`
}

type ReferenceLink struct {
	Label string `json:"label,omitempty"`
	Href  string `json:"href,omitempty"`
}
