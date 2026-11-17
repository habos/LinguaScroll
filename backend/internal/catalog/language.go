package catalog

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/habos/linguascroll/backend/internal/langdetect"
	"github.com/habos/linguascroll/backend/internal/youtube"
)

// LanguageRules is what ClassifyLanguage needs to decide whether a clip is
// in Target. Everything language-specific comes from data: Script is the
// target's languages.script, seed channels come from youtube_sources and the
// thresholds from app_config.
type LanguageRules struct {
	Target string
	// Script is languages.script: a Unicode script name ("Hangul",
	// "Cyrillic", "Devanagari", ...) or "Japanese" (kana plus Han). Empty
	// or unknown skips the script checks.
	Script        string
	SeedChannels  map[string]bool
	Detector      langdetect.Detector
	MinConfidence float64 // youtube.min_language_confidence
	// MinScriptShare is youtube.min_script_share: the share of words in
	// the title (or the description when the title has none) that must be
	// in Script before text detection may keep a clip.
	MinScriptShare float64
}

// Classification is how a clip's language was decided.
type Classification struct {
	Language   string
	Source     string // audio_tag | seed_channel | text_detect
	Confidence float64
}

// ClassifyLanguage applies the V3 language rules; ok=false means drop the
// clip. In order:
//
//  0. A title that is (almost) only hashtags, mentions and links
//     ("#한국바이럴 #웃긴영상 #쇼츠") is dropped unless the channel is a seed
//     channel: reupload channels use such titles and often set a false
//     audio language too.
//  1. Audio tag set: it must match the target, and the title must not be
//     written mainly in another non-Latin script (a Hindi title on a clip
//     tagged Korean is a wrong tag).
//  2. Seed channel: trusted.
//  3. Otherwise text: the declared metadata language (defaultLanguage), when
//     set, must match; hashtags, mentions and links are ignored; the
//     detector must agree with enough confidence; and enough of the title's
//     words must be in the target's script. A clip that only mentions the
//     language ("Korean girl reacts #korean #한국") fails here.
func ClassifyLanguage(v youtube.Video, r LanguageRules) (Classification, bool) {
	if !r.SeedChannels[v.ChannelID] && hashtagTitle(v.Title) {
		return Classification{}, false
	}
	if v.DefaultAudioLanguage != "" {
		if !sameLanguage(v.DefaultAudioLanguage, r.Target) || foreignScript(v.Title, r.Script) {
			return Classification{}, false
		}
		return Classification{r.Target, "audio_tag", 1}, true
	}
	if r.SeedChannels[v.ChannelID] {
		return Classification{r.Target, "seed_channel", 1}, true
	}
	if v.DefaultLanguage != "" && !sameLanguage(v.DefaultLanguage, r.Target) {
		return Classification{}, false
	}
	if r.Detector == nil {
		return Classification{}, false
	}
	title, desc := CleanText(v.Title), CleanText(shortDescription(v.Description))
	lang, conf := r.Detector.Detect(strings.TrimSpace(title + "\n" + desc))
	if lang != r.Target || conf < r.MinConfidence {
		return Classification{}, false
	}
	text := title
	if share, words := ScriptShare(title, r.Script); words == 0 || share < 0 {
		text = desc
	}
	if share, _ := ScriptShare(text, r.Script); share >= 0 && share < r.MinScriptShare {
		return Classification{}, false
	}
	return Classification{r.Target, "text_detect", conf}, true
}

func sameLanguage(a, b string) bool {
	return strings.EqualFold(primarySubtag(a), primarySubtag(b))
}

var hashtag = regexp.MustCompile(`[#＃]\S+`)

// hashtagTitle reports a title with no words of its own, or at most two
// words lost among three or more hashtags.
func hashtagTitle(title string) bool {
	n := len(words(CleanText(title)))
	return n == 0 || (n <= 2 && len(hashtag.FindAllString(title, -1)) >= 3)
}

var noise = regexp.MustCompile(`(?i)(https?://\S+|www\.\S+|[#＃@]\S+)`)

// CleanText drops hashtags, @mentions and links: creators stuff those with
// popular words in any language, so they say little about the clip's own
// language.
func CleanText(s string) string {
	return strings.Join(strings.Fields(noise.ReplaceAllString(s, " ")), " ")
}

// scriptTables returns the Unicode ranges for a languages.script value, or
// nil when it isn't known.
func scriptTables(script string) []*unicode.RangeTable {
	switch script {
	case "":
		return nil
	case "Japanese":
		return []*unicode.RangeTable{unicode.Hiragana, unicode.Katakana, unicode.Han}
	case "Hangul":
		return []*unicode.RangeTable{unicode.Hangul, unicode.Han}
	}
	if t, ok := unicode.Scripts[script]; ok {
		return []*unicode.RangeTable{t}
	}
	return nil
}

// scriptOf names the Unicode script of a letter (kana and Han count as one
// "CJK" group so mixed Japanese words stay one word).
func scriptOf(r rune) string {
	switch {
	case unicode.Is(unicode.Latin, r):
		return "Latin"
	case unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han):
		return "CJK"
	}
	for name, t := range unicode.Scripts {
		if unicode.Is(t, r) {
			return name
		}
	}
	return "Other"
}

// words splits text into runs of letters of one script. CJK text without
// spaces counts one word per run, which is fine for a share.
func words(s string) []string {
	var out []string
	var cur []rune
	curScript := ""
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Mc, r) {
			flush()
			curScript = ""
			continue
		}
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) { // vowel signs belong to the word
			if len(cur) > 0 {
				cur = append(cur, r)
			}
			continue
		}
		sc := scriptOf(r)
		if sc != curScript {
			flush()
			curScript = sc
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

func inTables(w string, tables []*unicode.RangeTable) bool {
	for _, r := range w {
		if unicode.IsLetter(r) {
			return unicode.In(r, tables...)
		}
	}
	return false
}

// ScriptShare is the share of words in text written in script, and the
// number of words. The share is -1 when script is unknown (no check).
func ScriptShare(text, script string) (share float64, n int) {
	tables := scriptTables(script)
	ws := words(text)
	if tables == nil {
		return -1, len(ws)
	}
	if len(ws) == 0 {
		return 0, 0
	}
	in := 0
	for _, w := range ws {
		if inTables(w, tables) {
			in++
		}
	}
	return float64(in) / float64(len(ws)), len(ws)
}

// foreignScript reports whether the title is written mostly in a non-Latin
// script other than the target's. Latin titles are common on clips in any
// language, so they never count as foreign.
func foreignScript(title, script string) bool {
	tables := scriptTables(script)
	if tables == nil {
		return false
	}
	ws := words(CleanText(title))
	foreign := 0
	for _, w := range ws {
		if !inTables(w, tables) && !inTables(w, []*unicode.RangeTable{unicode.Latin}) {
			foreign++
		}
	}
	return foreign >= 2 && float64(foreign)/float64(len(ws)) > 0.5
}
