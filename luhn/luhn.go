package luhn

import "strings"

type Alphabet string

var (
	Base32        Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567="
	Base32Trimmed Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
)

func (a Alphabet) Generate(s string) rune {
	factor := 1
	sum := 0
	n := len(a)

	for i := range s {
		codepoint := strings.IndexByte(string(a), s[i])
		addend := factor * codepoint
		if factor == 2 {
			factor = 1
		} else {
			factor = 2
		}
		addend = (addend / n) + (addend % n)
		sum += addend
	}
	remainder := sum % n
	checkCodepoint := (n - remainder) % n
	return rune(a[checkCodepoint])
}

func (a Alphabet) Validate(s string) bool {
	t := s[:len(s)-1]
	c := a.Generate(t)
	return rune(s[len(s)-1]) == c
}
