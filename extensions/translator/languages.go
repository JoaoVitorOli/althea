package translator

import (
	"strings"
	"unicode"
)

// pivot is the language other pairs go through.
const pivot = "en"

// languageOf reduces the user's language, as "pt-BR", to the Translator's, as "pt".
func languageOf(tag string) string {
	lang, _, _ := strings.Cut(tag, "-")
	return strings.ToLower(lang)
}

// scripts tell a language by its alphabet, where only one of the Translator's uses it.
var scripts = []struct {
	table *unicode.RangeTable
	lang  string
}{
	{unicode.Greek, "el"},
	{unicode.Hebrew, "he"},
	{unicode.Arabic, "fa"},
	{unicode.Devanagari, "hi"},
	{unicode.Bengali, "bn"},
	{unicode.Gujarati, "gu"},
	{unicode.Kannada, "kn"},
	{unicode.Malayalam, "ml"},
	{unicode.Tamil, "ta"},
	{unicode.Telugu, "te"},
}

// words are common words that tell languages with the same alphabet apart.
var words = map[string][]string{
	"en": {"the", "and", "is", "of", "to", "in", "that", "it", "for", "you", "with", "this", "was", "are", "not", "have"},
	"pt": {"o", "a", "os", "as", "de", "que", "e", "em", "um", "uma", "para", "com", "não", "é", "do", "da", "no", "na", "você", "isso", "mais", "por"},
	"es": {"el", "la", "los", "las", "de", "que", "y", "en", "un", "una", "para", "con", "no", "es", "del", "por", "está", "pero", "muy", "yo"},
	"fr": {"le", "la", "les", "de", "des", "et", "en", "un", "une", "pour", "avec", "pas", "est", "que", "je", "vous", "dans", "qui", "sur", "ce"},
	"de": {"der", "die", "das", "und", "ist", "nicht", "ein", "eine", "mit", "für", "auf", "zu", "den", "von", "ich", "sie", "es", "im", "dem"},
	"it": {"il", "la", "le", "di", "che", "e", "un", "una", "per", "con", "non", "è", "del", "della", "sono", "gli", "lo", "questo", "ma"},
	"nl": {"de", "het", "een", "en", "van", "is", "niet", "dat", "op", "voor", "met", "ik", "je", "te", "zijn", "maar", "aan"},
	"pl": {"i", "w", "nie", "na", "się", "że", "jest", "do", "to", "z", "jak", "co", "ale", "tak", "dla", "czy"},
	"sv": {"och", "det", "att", "är", "en", "ett", "som", "på", "för", "med", "inte", "jag", "av", "den", "har", "till"},
	"da": {"og", "det", "at", "er", "en", "et", "som", "på", "for", "med", "ikke", "jeg", "af", "den", "har", "til"},
	"ro": {"și", "este", "de", "la", "în", "un", "o", "cu", "nu", "pe", "care", "pentru", "mai", "sunt"},
	"tr": {"ve", "bir", "bu", "için", "ile", "de", "da", "ne", "çok", "ben", "değil", "gibi", "var", "daha"},
	"cs": {"a", "je", "v", "na", "se", "že", "to", "s", "z", "jako", "ale", "pro", "není", "jsem"},
	"id": {"yang", "dan", "di", "ini", "itu", "dengan", "untuk", "tidak", "dari", "saya", "akan", "ada", "pada"},
}

// wordIndex finds the languages of a word.
var wordIndex = func() map[string][]string {
	index := map[string][]string{}
	for lang, list := range words {
		for _, w := range list {
			index[w] = append(index[w], lang)
		}
	}
	return index
}()

// Detect guesses the language of text, cheaply: by its alphabet, or by counting common words. It
// is a guess the user can correct, so it stays small rather than perfect; "" when it cannot tell.
func Detect(text string) string {
	cyrillic, letters := 0, 0
	scriptCount := make([]int, len(scripts))
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.Is(unicode.Cyrillic, r) {
			cyrillic++
		}
		for i, s := range scripts {
			if unicode.Is(s.table, r) {
				scriptCount[i]++
			}
		}
	}
	if letters == 0 {
		return ""
	}
	for i, n := range scriptCount {
		if n*2 > letters {
			return scripts[i].lang
		}
	}
	if cyrillic*2 > letters {
		return "ru" // the commonest of the Cyrillic ones
	}
	score := map[string]float64{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		langs := wordIndex[w]
		for _, lang := range langs {
			score[lang] += 1 / float64(len(langs)) // a word many languages share says less
		}
	}
	best, top := "", 0.0
	for lang, s := range score {
		if s > top || (s == top && lang < best) {
			best, top = lang, s
		}
	}
	return best
}
