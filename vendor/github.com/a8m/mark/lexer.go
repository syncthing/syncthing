package mark

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// type position
type Pos int

// itemType identifies the type of lex items.
type itemType int

// Item represent a token or text string returned from the scanner
type item struct {
	typ itemType // The type of this item.
	pos Pos      // The starting position, in bytes, of this item in the input string.
	val string   // The value of this item.
}

const eof = -1 // Zero value so closed channel delivers EOF

const (
	itemError itemType = iota // Error occurred; value is text of error
	itemEOF
	itemNewLine
	itemHTML
	itemHeading
	itemLHeading
	itemBlockQuote
	itemList
	itemListItem
	itemLooseItem
	itemCodeBlock
	itemGfmCodeBlock
	itemHr
	itemTable
	itemLpTable
	itemTableRow
	itemTableCell
	itemStrong
	itemItalic
	itemStrike
	itemCode
	itemLink
	itemDefLink
	itemRefLink
	itemAutoLink
	itemGfmLink
	itemImage
	itemRefImage
	itemText
	itemBr
	itemPipe
	itemIndent
)

// stateFn represents the state of the scanner as a function that returns the next state.
type stateFn func(*lexer) stateFn

// Lexer interface, used to composed it inside the parser
type Lexer interface {
	nextItem() item
}

// lexer holds the state of the scanner.
type lexer struct {
	input   string    // the string being scanned
	state   stateFn   // the next lexing function to enter
	pos     Pos       // current position in the input
	start   Pos       // start position of this item
	width   Pos       // width of last rune read from input
	lastPos Pos       // position of most recent item returned by nextItem
	items   chan item // channel of scanned items
}

// lex creates a new lexer for the input string.
func lex(input string) *lexer {
	l := &lexer{
		input: input,
		items: make(chan item),
	}
	go l.run()
	return l
}

// lexInline create a new lexer for one phase lexing(inline blocks).
func lexInline(input string) *lexer {
	l := &lexer{
		input: input,
		items: make(chan item),
	}
	go l.lexInline()
	return l
}

// run runs the state machine for the lexer.
func (l *lexer) run() {
	for l.state = lexAny; l.state != nil; {
		l.state = l.state(l)
	}
	close(l.items)
}

// next return the next rune in the input
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
	l.pos += l.width
	return r
}

// lexAny scanner is kind of forwarder, it get the current char in the text
// and forward it to the appropriate scanner based on some conditions.
func lexAny(l *lexer) stateFn {
	switch r := l.peek(); r {
	case '*', '-', '_':
		return lexHr
	case '+', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return lexList
	case '<':
		return lexHTML
	case '>':
		return lexBlockQuote
	case '[':
		return lexDefLink
	case '#':
		return lexHeading
	case '`', '~':
		return lexGfmCode
	case ' ':
		if reCodeBlock.MatchString(l.input[l.pos:]) {
			return lexCode
		} else if reGfmCode.MatchString(l.input[l.pos:]) {
			return lexGfmCode
		}
		// Keep moving forward until we get all the indentation size
		for ; r == l.peek(); r = l.next() {
		}
		l.emit(itemIndent)
		return lexAny
	case '|':
		if m := reTable.itemLp.MatchString(l.input[l.pos:]); m {
			l.emit(itemLpTable)
			return lexTable
		}
		fallthrough
	default:
		if m := reTable.item.MatchString(l.input[l.pos:]); m {
			l.emit(itemTable)
			return lexTable
		}
		return lexText
	}
}

// lexHeading test if the current text position is an heading item.
// is so, it will emit an item and return back to lenAny function
// else, lex it as a simple text value
func lexHeading(l *lexer) stateFn {
	if m := reHeading.FindString(l.input[l.pos:]); m != "" {
		l.pos += Pos(len(m))
		l.emit(itemHeading)
		return lexAny
	}
	return lexText
}

// lexHr test if the current text position is an horizontal rules item.
// is so, it will emit an horizontal rule item and return back to lenAny function
// else, forward it to lexList function
func lexHr(l *lexer) stateFn {
	if match := reHr.FindString(l.input[l.pos:]); match != "" {
		l.pos += Pos(len(match))
		l.emit(itemHr)
		return lexAny
	}
	return lexList
}

// lexGfmCode test if the current text position is start of GFM code-block item.
// if so, it will generate regexp based on the fence type[`~] and it length.
// it scan until the end, and then emit the code-block item and return back to the
// lenAny forwarder.
// else, lex it as a simple inline text.
func lexGfmCode(l *lexer) stateFn {
	if match := reGfmCode.FindStringSubmatch(l.input[l.pos:]); len(match) != 0 {
		l.pos += Pos(len(match[0]))
		fence := match[2]
		// Generate Regexp based on fence type[`~] and length
		reGfmEnd := reGfmCode.endGen(fence[0:1], len(fence))
		infoContainer := reGfmEnd.FindStringSubmatch(l.input[l.pos:])
		l.pos += Pos(len(infoContainer[0]))
		infoString := infoContainer[1]
		// Remove leading and trailing spaces
		if indent := len(match[1]); indent > 0 {
			reSpace := reSpaceGen(indent)
			infoString = reSpace.ReplaceAllString(infoString, "")
		}
		l.emit(itemGfmCodeBlock, match[0]+infoString)
		return lexAny
	}
	return lexText
}

// lexCode scans code block.
func lexCode(l *lexer) stateFn {
	match := reCodeBlock.FindString(l.input[l.pos:])
	l.pos += Pos(len(match))
	l.emit(itemCodeBlock)
	return lexAny
}

// lexText scans until end-of-line(\n)
func lexText(l *lexer) stateFn {
	// Drain text before emitting
	emit := func(item itemType, pos Pos) {
		if l.pos > l.start {
			l.emit(itemText)
		}
		l.pos += pos
		l.emit(item)
	}
Loop:
	for {
		switch r := l.peek(); r {
		case eof:
			emit(itemEOF, Pos(0))
			break Loop
		case '\n':
			// CM 4.4: An indented code block cannot interrupt a paragraph.
			if l.pos > l.start && strings.HasPrefix(l.input[l.pos+1:], "    ") {
				l.next()
				continue
			}
			emit(itemNewLine, l.width)
			break Loop
		default:
			// Test for Setext-style headers
			if m := reLHeading.FindString(l.input[l.pos:]); m != "" {
				emit(itemLHeading, Pos(len(m)))
				break Loop
			}
			l.next()
		}
	}
	return lexAny
}

// backup steps back one rune. Can only be called once per call of next.
func (l *lexer) backup() {
	l.pos -= l.width
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// emit passes an item back to the client.
func (l *lexer) emit(t itemType, s ...string) {
	if len(s) == 0 {
		s = append(s, l.input[l.start:l.pos])
	}
	l.items <- item{t, l.start, s[0]}
	l.start = l.pos
}

// lexItem return the next item token, called by the parser.
func (l *lexer) nextItem() item {
	item := <-l.items
	l.lastPos = l.pos
	return item
}

// One phase lexing(inline reason)
func (l *lexer) lexInline() {
	escape := regexp.MustCompile("^\\\\([\\`*{}\\[\\]()#+\\-.!_>~|])")
	// Drain text before emitting
	emit := func(item itemType, pos int) {
		if l.pos > l.start {
			l.emit(itemText)
		}
		l.pos += Pos(pos)
		l.emit(item)
	}
Loop:
	for {
		switch r := l.peek(); r {
		case eof:
			if l.pos > l.start {
				l.emit(itemText)
			}
			break Loop
		// backslash escaping
		case '\\':
			if m := escape.FindStringSubmatch(l.input[l.pos:]); len(m) != 0 {
				if l.pos > l.start {
					l.emit(itemText)
				}
				l.pos += Pos(len(m[0]))
				l.emit(itemText, m[1])
				break
			}
			fallthrough
		case ' ':
			if m := reBr.FindString(l.input[l.pos:]); m != "" {
				// pos - length of new-line
				emit(itemBr, len(m))
				break
			}
			l.next()
		case '_', '*', '~', '`':
			input := l.input[l.pos:]
			// Strong
			if m := reStrong.FindString(input); m != "" {
				emit(itemStrong, len(m))
				break
			}
			// Italic
			if m := reItalic.FindString(input); m != "" {
				emit(itemItalic, len(m))
				break
			}
			// Strike
			if m := reStrike.FindString(input); m != "" {
				emit(itemStrike, len(m))
				break
			}
			// InlineCode
			if m := reCode.FindString(input); m != "" {
				emit(itemCode, len(m))
				break
			}
			l.next()
		// itemLink, itemImage, itemRefLink, itemRefImage
		case '[', '!':
			input := l.input[l.pos:]
			if m := reLink.FindString(input); m != "" {
				pos := len(m)
				if r == '[' {
					emit(itemLink, pos)
				} else {
					emit(itemImage, pos)
				}
				break
			}
			if m := reRefLink.FindString(input); m != "" {
				pos := len(m)
				if r == '[' {
					emit(itemRefLink, pos)
				} else {
					emit(itemRefImage, pos)
				}
				break
			}
			l.next()
		// itemAutoLink, htmlBlock
		case '<':
			if m := reAutoLink.FindString(l.input[l.pos:]); m != "" {
				emit(itemAutoLink, len(m))
				break
			}
			if match, res := l.matchHTML(l.input[l.pos:]); match {
				emit(itemHTML, len(res))
				break
			}
			l.next()
		default:
			if m := reGfmLink.FindString(l.input[l.pos:]); m != "" {
				emit(itemGfmLink, len(m))
				break
			}
			l.next()
		}
	}
	close(l.items)
}

// lexHTML.
func lexHTML(l *lexer) stateFn {
	if match, res := l.matchHTML(l.input[l.pos:]); match {
		l.pos += Pos(len(res))
		l.emit(itemHTML)
		return lexAny
	}
	return lexText
}

// Test if the given input is match the HTML pattern(blocks only)
func (l *lexer) matchHTML(input string) (bool, string) {
	if m := reHTML.comment.FindString(input); m != "" {
		return true, m
	}
	if m := reHTML.item.FindStringSubmatch(input); len(m) != 0 {
		el, name := m[0], m[1]
		// if name is a span... is a text
		if reHTML.span.MatchString(name) {
			return false, ""
		}
		// if it's a self-closed html element, but not a itemAutoLink
		if strings.HasSuffix(el, "/>") && !reAutoLink.MatchString(el) {
			return true, el
		}
		if name == reHTML.CDATA_OPEN {
			name = reHTML.CDATA_CLOSE
		}
		reEndTag := reHTML.endTagGen(name)
		if m := reEndTag.FindString(input); m != "" {
			return true, m
		}
	}
	return false, ""
}

// lexDefLink scans link definition
func lexDefLink(l *lexer) stateFn {
	if m := reDefLink.FindString(l.input[l.pos:]); m != "" {
		l.pos += Pos(len(m))
		l.emit(itemDefLink)
		return lexAny
	}
	return lexText
}

// lexList scans ordered and unordered lists.
func lexList(l *lexer) stateFn {
	match, items := l.matchList(l.input[l.pos:])
	if !match {
		return lexText
	}
	var space int
	var typ itemType
	for i, item := range items {
		// Emit itemList on the first loop
		if i == 0 {
			l.emit(itemList, reList.marker.FindStringSubmatch(item)[1])
		}
		// Initialize each loop
		typ = itemListItem
		space = len(item)
		l.pos += Pos(space)
		item = reList.marker.ReplaceAllString(item, "")
		// Indented
		if strings.Contains(item, "\n ") {
			space -= len(item)
			reSpace := reSpaceGen(space)
			item = reSpace.ReplaceAllString(item, "")
		}
		// If current is loose
		for _, l := range reList.loose.FindAllString(item, -1) {
			if len(strings.TrimSpace(l)) > 0 || i != len(items)-1 {
				typ = itemLooseItem
				break
			}
		}
		// or previous
		if typ != itemLooseItem && i > 0 && strings.HasSuffix(items[i-1], "\n\n") {
			typ = itemLooseItem
		}
		l.emit(typ, strings.TrimSpace(item))
	}
	return lexAny
}

func (l *lexer) matchList(input string) (bool, []string) {
	var res []string
	reItem := reList.item
	if !reItem.MatchString(input) {
		return false, res
	}
	// First item
	m := reItem.FindStringSubmatch(input)
	item, depth := m[0], len(m[1])
	input = input[len(item):]
	// Loop over the input
	for len(input) > 0 {
		// Count new-lines('\n')
		if m := reList.scanNewLine(input); m != "" {
			item += m
			input = input[len(m):]
			if len(m) >= 2 || !reItem.MatchString(input) && !strings.HasPrefix(input, " ") {
				break
			}
		}
		// DefLink or hr
		if reDefLink.MatchString(input) || reHr.MatchString(input) {
			break
		}
		// It's list in the same depth
		if m := reItem.FindStringSubmatch(input); len(m) > 0 && len(m[1]) == depth {
			if item != "" {
				res = append(res, item)
			}
			item = m[0]
			input = input[len(item):]
		} else {
			m := reList.scanLine(input)
			item += m
			input = input[len(m):]
		}
	}
	// Drain res
	if item != "" {
		res = append(res, item)
	}
	return true, res
}

// Test if the given input match blockquote
func (l *lexer) matchBlockQuote(input string) (bool, string) {
	match := reBlockQuote.FindString(input)
	if match == "" {
		return false, match
	}
	lines := strings.Split(match, "\n")
	for i, line := range lines {
		// if line is a link-definition or horizontal role, we cut the match until this point
		if reDefLink.MatchString(line) || reHr.MatchString(line) {
			match = strings.Join(lines[0:i], "\n")
			break
		}
	}
	return true, match
}

// lexBlockQuote
func lexBlockQuote(l *lexer) stateFn {
	if match, res := l.matchBlockQuote(l.input[l.pos:]); match {
		l.pos += Pos(len(res))
		l.emit(itemBlockQuote)
		return lexAny
	}
	return lexText
}

// lexTable
func lexTable(l *lexer) stateFn {
	re := reTable.item
	if l.peek() == '|' {
		re = reTable.itemLp
	}
	table := re.FindStringSubmatch(l.input[l.pos:])
	l.pos += Pos(len(table[0]))
	l.start = l.pos
	// Ignore the first match, and flat all rows(by splitting \n)
	rows := append(table[1:3], strings.Split(table[3], "\n")...)
	for _, row := range rows {
		if row == "" {
			continue
		}
		l.emit(itemTableRow)
		rawCells := reTable.trim(row, "")
		cells := reTable.split(rawCells, -1)
		// Emit cells in the current row
		for _, cell := range cells {
			l.emit(itemTableCell, cell)
		}
	}
	return lexAny
}
