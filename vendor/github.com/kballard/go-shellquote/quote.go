package shellquote

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Join quotes each argument and joins them with a space.
// If passed to /bin/sh, the resulting string will be split back into the
// original arguments.
func Join(args ...string) string {
	var buf bytes.Buffer
	for i, arg := range args {
		if i != 0 {
			buf.WriteByte(' ')
		}
		quote(arg, &buf)
	}
	return buf.String()
}

const (
	specialChars      = "\\'\"`${[|&;<>()*?!"
	extraSpecialChars = " \t\n"
	prefixChars       = "~"
)

func quote(word string, buf *bytes.Buffer) {
	// We want to try to produce a "nice" output. As such, we will
	// backslash-escape most characters, but if we encounter a space, or if we
	// encounter an extra-special char (which doesn't work with
	// backslash-escaping) we switch over to quoting the whole word. We do this
	// with a space because it's typically easier for people to read multi-word
	// arguments when quoted with a space rather than with ugly backslashes
	// everywhere.
	origLen := buf.Len()

	if len(word) == 0 {
		// oops, no content
		buf.WriteString("''")
		return
	}

	cur, prev := word, word
	atStart := true
	for len(cur) > 0 {
		c, l := utf8.DecodeRuneInString(cur)
		cur = cur[l:]
		if strings.ContainsRune(specialChars, c) || (atStart && strings.ContainsRune(prefixChars, c)) {
			// copy the non-special chars up to this point
			if len(cur) < len(prev) {
				buf.WriteString(prev[0 : len(prev)-len(cur)-l])
			}
			buf.WriteByte('\\')
			buf.WriteRune(c)
			prev = cur
		} else if strings.ContainsRune(extraSpecialChars, c) {
			// start over in quote mode
			buf.Truncate(origLen)
			goto quote
		}
		atStart = false
	}
	if len(prev) > 0 {
		buf.WriteString(prev)
	}
	return

quote:
	// quote mode
	// Use single-quotes, but if we find a single-quote in the word, we need
	// to terminate the string, emit an escaped quote, and start the string up
	// again
	inQuote := false
	for len(word) > 0 {
		i := strings.IndexRune(word, '\'')
		if i == -1 {
			break
		}
		if i > 0 {
			if !inQuote {
				buf.WriteByte('\'')
				inQuote = true
			}
			buf.WriteString(word[0:i])
		}
		word = word[i+1:]
		if inQuote {
			buf.WriteByte('\'')
			inQuote = false
		}
		buf.WriteString("\\'")
	}
	if len(word) > 0 {
		if !inQuote {
			buf.WriteByte('\'')
		}
		buf.WriteString(word)
		buf.WriteByte('\'')
	}
}
