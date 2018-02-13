package pb

import (
	"github.com/mattn/go-runewidth"
	"regexp"
)

// Finds the control character sequences (like colors)
var ctrlFinder = regexp.MustCompile("\x1b\x5b[0-9]+\x6d")

func escapeAwareRuneCountInString(s string) int {
	n := runewidth.StringWidth(s)
	for _, sm := range ctrlFinder.FindAllString(s, -1) {
		n -= runewidth.StringWidth(sm)
	}
	return n
}
