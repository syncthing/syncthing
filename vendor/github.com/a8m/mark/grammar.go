package mark

import (
	"fmt"
	"regexp"
)

// Block Grammar
var (
	reHr         = regexp.MustCompile(`^(?:(?:\* *){3,}|(?:_ *){3,}|(?:- *){3,}) *(?:\n+|$)`)
	reHeading    = regexp.MustCompile(`^ *(#{1,6})(?: +#*| +([^\n]*?)|)(?: +#*|) *(?:\n|$)`)
	reLHeading   = regexp.MustCompile(`^([^\n]+?) *\n {0,3}(=|-){1,} *(?:\n+|$)`)
	reBlockQuote = regexp.MustCompile(`^ *>[^\n]*(\n[^\n]+)*\n*`)
	reDefLink    = regexp.MustCompile(`(?s)^ *\[([^\]]+)\]: *\n? *<?([^\s>]+)>?(?: *\n? *["'(](.+?)['")])? *(?:\n+|$)`)
	reSpaceGen   = func(i int) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`(?m)^ {1,%d}`, i))
	}
)

var reList = struct {
	item, marker, loose   *regexp.Regexp
	scanLine, scanNewLine func(src string) string
}{
	regexp.MustCompile(`^( *)(?:[*+-]|\d{1,9}\.) (.*)(?:\n|)`),
	regexp.MustCompile(`^ *([*+-]|\d+\.) +`),
	regexp.MustCompile(`(?m)\n\n(.*)`),
	regexp.MustCompile(`^(.*)(?:\n|)`).FindString,
	regexp.MustCompile(`^\n{1,}`).FindString,
}

var reCodeBlock = struct {
	*regexp.Regexp
	trim func(src, repl string) string
}{
	regexp.MustCompile(`^( {4}[^\n]+(?: *\n)*)+`),
	regexp.MustCompile("(?m)^( {0,4})").ReplaceAllLiteralString,
}

var reGfmCode = struct {
	*regexp.Regexp
	endGen func(end string, i int) *regexp.Regexp
}{
	regexp.MustCompile("^( {0,3})([`~]{3,}) *(\\S*)?(?:.*)"),
	func(end string, i int) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`(?s)(.*?)(?:((?m)^ {0,3}%s{%d,} *$)|$)`, end, i))
	},
}

var reTable = struct {
	item, itemLp *regexp.Regexp
	split        func(s string, n int) []string
	trim         func(src, repl string) string
}{
	regexp.MustCompile(`^ *(\S.*\|.*)\n *([-:]+ *\|[-| :]*)\n((?:.*\|.*(?:\n|$))*)\n*`),
	regexp.MustCompile(`(^ *\|.+)\n( *\| *[-:]+[-| :]*)\n((?: *\|.*(?:\n|$))*)\n*`),
	regexp.MustCompile(` *\| *`).Split,
	regexp.MustCompile(`^ *\| *| *\| *$`).ReplaceAllString,
}

var reHTML = struct {
	CDATA_OPEN, CDATA_CLOSE  string
	item, comment, tag, span *regexp.Regexp
	endTagGen                func(tag string) *regexp.Regexp
}{
	`![CDATA[`,
	"?\\]\\]",
	regexp.MustCompile(`^<(\w+|!\[CDATA\[)(?:"[^"]*"|'[^']*'|[^'">])*?>`),
	regexp.MustCompile(`(?sm)<!--.*?-->`),
	regexp.MustCompile(`^<!--.*?-->|^<\/?\w+(?:"[^"]*"|'[^']*'|[^'">])*?>`),
	// TODO: Add all span-tags and move to config.
	regexp.MustCompile(`^(a|em|strong|small|s|q|data|time|code|sub|sup|i|b|u|span|br|del|img)$`),
	func(tag string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`(?s)(.+?)<\/%s> *`, tag))
	},
}

// Inline Grammar
var (
	reBr        = regexp.MustCompile(`^(?: {2,}|\\)\n`)
	reLinkText  = `(?:\[[^\]]*\]|[^\[\]]|\])*`
	reLinkHref  = `\s*<?(.*?)>?(?:\s+['"\(](.*?)['"\)])?\s*`
	reGfmLink   = regexp.MustCompile(`^(https?:\/\/[^\s<]+[^<.,:;"')\]\s])`)
	reLink      = regexp.MustCompile(fmt.Sprintf(`(?s)^!?\[(%s)\]\(%s\)`, reLinkText, reLinkHref))
	reAutoLink  = regexp.MustCompile(`^<([^ >]+(@|:\/)[^ >]+)>`)
	reRefLink   = regexp.MustCompile(`^!?\[((?:\[[^\]]*\]|[^\[\]]|\])*)\](?:\s*\[([^\]]*)\])?`)
	reImage     = regexp.MustCompile(fmt.Sprintf(`(?s)^!?\[(%s)\]\(%s\)`, reLinkText, reLinkHref))
	reCode      = regexp.MustCompile("(?s)^`{1,2}\\s*(.*?[^`])\\s*`{1,2}")
	reStrike    = regexp.MustCompile(`(?s)^~{2}(.+?)~{2}`)
	reEmphasise = `(?s)^_{%[1]d}(\S.*?_*)_{%[1]d}|^\*{%[1]d}(\S.*?\**)\*{%[1]d}`
	reItalic    = regexp.MustCompile(fmt.Sprintf(reEmphasise, 1))
	reStrong    = regexp.MustCompile(fmt.Sprintf(reEmphasise, 2))
)
