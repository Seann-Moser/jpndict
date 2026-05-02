package translate

import (
	"fmt"
	"sort"
	"strings"
)

func languageName(lang Language) string {
	switch lang {
	case LanguageEnglish:
		return "English"
	case LanguageJapanese:
		return "Japanese"
	default:
		return string(lang)
	}
}

func BuildTranslationSystemPrompt(r *Request) string {
	var b strings.Builder

	b.WriteString("You are a translation engine for visual novels and Japanese games.\n")
	b.WriteString("Translate the user's text into ")
	b.WriteString(languageName(r.ToLanguage))
	b.WriteString(".\n\n")

	b.WriteString("Rules:\n")
	b.WriteString("- Output only the translated text.\n")
	b.WriteString("- Do not explain the translation.\n")
	b.WriteString("- Preserve names, honorifics, tone, and line breaks when useful.\n")
	b.WriteString("- Do not add information that is not present in the source.\n")

	if r.PreferLiteral {
		b.WriteString("- Prefer a literal translation over a localized rewrite.\n")
	} else {
		b.WriteString("- Prefer natural dialogue, but stay faithful to the source.\n")
	}

	if len(r.Glossary) > 0 {
		b.WriteString("\nGlossary:\n")

		keys := make([]string, 0, len(r.Glossary))
		for k := range r.Glossary {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- %s => %s\n", k, r.Glossary[k]))
		}
	}

	return b.String()
}

func BuildTranslationUserPrompt(r *Request) string {
	var b strings.Builder

	if cleanText(r.GameTitle) != "" {
		b.WriteString("Game: ")
		b.WriteString(r.GameTitle)
		b.WriteString("\n")
	}

	if cleanText(r.Scene) != "" {
		b.WriteString("Scene: ")
		b.WriteString(r.Scene)
		b.WriteString("\n")
	}

	if cleanText(r.Speaker) != "" {
		b.WriteString("Speaker: ")
		b.WriteString(r.Speaker)
		b.WriteString("\n")
	}

	if cleanText(r.SurroundingContext) != "" {
		b.WriteString("\nContext:\n")
		b.WriteString(r.SurroundingContext)
		b.WriteString("\n")
	}

	b.WriteString("\nText to translate:\n")
	b.WriteString(r.Text)

	return b.String()
}
