// Package english provides utilities to generate more user-friendly English output.
package english

import (
	"fmt"
	"strings"
)

// These are included because they are common technical terms.
var specialPlurals = map[string]string{
	"index":  "indices",
	"matrix": "matrices",
	"vertex": "vertices",
}

var sibilantEndings = []string{"s", "sh", "tch", "x"}

var isVowel = map[byte]bool{
	'A': true, 'E': true, 'I': true, 'O': true, 'U': true,
	'a': true, 'e': true, 'i': true, 'o': true, 'u': true,
}

// PluralWord builds the plural form of an English word.
// The simple English rules of regular pluralization will be used
// if the plural form is an empty string (i.e. not explicitly given).
// The special cases are not guaranteed to work for strings outside ASCII.
func PluralWord(quantity int, singular, plural string) string {
	if quantity == 1 {
		return singular
	}
	if plural != "" {
		return plural
	}
	if plural = specialPlurals[singular]; plural != "" {
		return plural
	}

	// We need to guess what the English plural might be.  Keep this
	// function simple!  It doesn't need to know about every possiblity;
	// only regular rules and the most common special cases.
	//
	// Reference: http://en.wikipedia.org/wiki/English_plural

	for _, ending := range sibilantEndings {
		if strings.HasSuffix(singular, ending) {
			return singular + "es"
		}
	}
	l := len(singular)
	if l >= 2 && singular[l-1] == 'o' && !isVowel[singular[l-2]] {
		return singular + "es"
	}
	if l >= 2 && singular[l-1] == 'y' && !isVowel[singular[l-2]] {
		return singular[:l-1] + "ies"
	}

	return singular + "s"
}

// Plural formats an integer and a string into a single pluralized string.
// The simple English rules of regular pluralization will be used
// if the plural form is an empty string (i.e. not explicitly given).
func Plural(quantity int, singular, plural string) string {
	return fmt.Sprintf("%d %s", quantity, PluralWord(quantity, singular, plural))
}

// WordSeries converts a list of words into a word series in English.
// It returns a string containing all the given words separated by commas,
// the coordinating conjunction, and a serial comma, as appropriate.
func WordSeries(words []string, conjunction string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	default:
		return fmt.Sprintf("%s %s %s", strings.Join(words[:len(words)-1], ", "), conjunction, words[len(words)-1])
	}
}

// OxfordWordSeries converts a list of words into a word series in English,
// using an Oxford comma (https://en.wikipedia.org/wiki/Serial_comma). It
// returns a string containing all the given words separated by commas, the
// coordinating conjunction, and a serial comma, as appropriate.
func OxfordWordSeries(words []string, conjunction string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return strings.Join(words, fmt.Sprintf(" %s ", conjunction))
	default:
		return fmt.Sprintf("%s, %s %s", strings.Join(words[:len(words)-1], ", "), conjunction, words[len(words)-1])
	}
}
