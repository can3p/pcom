package translate

import "github.com/abadojack/whatlanggo"

// DetectLanguage guesses the language text is written in, as ISO 639-1. It
// reports false when the guess isn't reliable (text too short or mixed) or
// the language has no ISO 639-1 code; the caller then stores null.
func DetectLanguage(text string) (string, bool) {
	info := whatlanggo.Detect(text)
	if !info.IsReliable() {
		return "", false
	}

	code := info.Lang.Iso6391()

	return code, code != ""
}
