package strings

import "strings"

func IndexAnyRunes(s string, rs []rune) int {
	for _, r := range rs {
		if i := strings.IndexRune(s, r); i != -1 {
			return i
		}
	}

	return -1
}
