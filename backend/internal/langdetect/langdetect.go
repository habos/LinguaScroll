// Package langdetect guesses the language of a clip's title, description
// and tags. The real detector is lingua-go limited to the launch languages.
package langdetect

import (
	"sort"
	"strings"
	"sync"

	"github.com/pemistahl/lingua-go"
)

// Detector returns a language code from the languages table and a
// confidence from 0 to 1. An empty code means no guess.
type Detector interface {
	Detect(text string) (lang string, confidence float64)
}

// codes maps lingua languages to our language codes.
var codes = map[lingua.Language]string{
	lingua.English: "en", lingua.Spanish: "es", lingua.French: "fr", lingua.Japanese: "ja",
	lingua.German: "de", lingua.Korean: "ko", lingua.Italian: "it", lingua.Hindi: "hi",
	lingua.Chinese: "zh-Hans", lingua.Russian: "ru", lingua.Arabic: "ar", lingua.Portuguese: "pt-BR",
	lingua.Turkish: "tr", lingua.Dutch: "nl", lingua.Swedish: "sv",
}

// Lingua is the lingua-go detector. It builds lazily on first use (models
// load in a few seconds) and is safe for concurrent use.
type Lingua struct {
	once sync.Once
	d    lingua.LanguageDetector
}

func (l *Lingua) detector() lingua.LanguageDetector {
	l.once.Do(func() {
		langs := make([]lingua.Language, 0, len(codes))
		for lang := range codes {
			langs = append(langs, lang)
		}
		l.d = lingua.NewLanguageDetectorBuilder().FromLanguages(langs...).Build()
	})
	return l.d
}

// Detect implements Detector.
func (l *Lingua) Detect(text string) (string, float64) {
	if strings.TrimSpace(text) == "" {
		return "", 0
	}
	vals := l.detector().ComputeLanguageConfidenceValues(text)
	if len(vals) == 0 {
		return "", 0
	}
	return codes[vals[0].Language()], vals[0].Value()
}

// Fake detects a language when the text contains one of its marker words
// (case-insensitive), with confidence 0.95; otherwise no guess. For tests
// and FAKE_VENDORS=1.
type Fake map[string][]string // language code -> marker words

// DefaultFake knows a few common words in es, fr and ja.
var DefaultFake = Fake{
	"es": {"hola", "español", "principiantes", "cómo", "qué", "para"},
	"fr": {"bonjour", "français", "comment", "pourquoi", "débutants"},
	"ja": {"日本語", "こんにちは", "です"},
}

// Detect implements Detector.
func (f Fake) Detect(text string) (string, float64) {
	t := strings.ToLower(text)
	langs := make([]string, 0, len(f))
	for lang := range f {
		langs = append(langs, lang)
	}
	sort.Strings(langs) // deterministic when text has several languages' words
	for _, lang := range langs {
		for _, w := range f[lang] {
			if strings.Contains(t, strings.ToLower(w)) {
				return lang, 0.95
			}
		}
	}
	return "", 0
}
