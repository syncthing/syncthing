package mark

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A Node is an element in the parse tree.
type Node interface {
	Type() NodeType
	Render() string
}

// NodeType identifies the type of a parse tree node.
type NodeType int

// Type returns itself and provides an easy default implementation
// for embedding in a Node. Embedded in all non-trivial Nodes.
func (t NodeType) Type() NodeType {
	return t
}

// Render function, used for overriding default rendering.
type RenderFn func(Node) string

const (
	NodeText       NodeType = iota // A plain text
	NodeParagraph                  // A Paragraph
	NodeEmphasis                   // An emphasis(strong, em, ...)
	NodeHeading                    // A heading (h1, h2, ...)
	NodeBr                         // A link break
	NodeHr                         // A horizontal rule
	NodeImage                      // An image
	NodeRefImage                   // A image reference
	NodeList                       // A list of ListItems
	NodeListItem                   // A list item node
	NodeLink                       // A link(href)
	NodeRefLink                    // A link reference
	NodeDefLink                    // A link definition
	NodeTable                      // A table of NodeRows
	NodeRow                        // A row of NodeCells
	NodeCell                       // A table-cell(td)
	NodeCode                       // A code block(wrapped with pre)
	NodeBlockQuote                 // A blockquote
	NodeHTML                       // An inline HTML
	NodeCheckbox                   // A checkbox
)

// ParagraphNode hold simple paragraph node contains text
// that may be emphasis.
type ParagraphNode struct {
	NodeType
	Pos
	Nodes []Node
}

// Render returns the html representation of ParagraphNode
func (n *ParagraphNode) Render() (s string) {
	for _, node := range n.Nodes {
		s += node.Render()
	}
	return wrap("p", s)
}

func (p *parse) newParagraph(pos Pos) *ParagraphNode {
	return &ParagraphNode{NodeType: NodeParagraph, Pos: pos}
}

// TextNode holds plain text.
type TextNode struct {
	NodeType
	Pos
	Text string
}

// Render returns the string representation of TexNode
func (n *TextNode) Render() string {
	return n.Text
}

func (p *parse) newText(pos Pos, text string) *TextNode {
	return &TextNode{NodeType: NodeText, Pos: pos, Text: p.text(text)}
}

// HTMLNode holds the raw html source.
type HTMLNode struct {
	NodeType
	Pos
	Src string
}

// Render returns the src of the HTMLNode
func (n *HTMLNode) Render() string {
	return n.Src
}

func (p *parse) newHTML(pos Pos, src string) *HTMLNode {
	return &HTMLNode{NodeType: NodeHTML, Pos: pos, Src: src}
}

// HrNode represents horizontal rule
type HrNode struct {
	NodeType
	Pos
}

// Render returns the html representation of hr.
func (n *HrNode) Render() string {
	return "<hr>"
}

func (p *parse) newHr(pos Pos) *HrNode {
	return &HrNode{NodeType: NodeHr, Pos: pos}
}

// BrNode represents a link-break element.
type BrNode struct {
	NodeType
	Pos
}

// Render returns the html representation of line-break.
func (n *BrNode) Render() string {
	return "<br>"
}

func (p *parse) newBr(pos Pos) *BrNode {
	return &BrNode{NodeType: NodeBr, Pos: pos}
}

// EmphasisNode holds plain-text wrapped with style.
// (strong, em, del, code)
type EmphasisNode struct {
	NodeType
	Pos
	Style itemType
	Nodes []Node
}

// Tag return the tagName based on the Style field.
func (n *EmphasisNode) Tag() (s string) {
	switch n.Style {
	case itemStrong:
		s = "strong"
	case itemItalic:
		s = "em"
	case itemStrike:
		s = "del"
	case itemCode:
		s = "code"
	}
	return
}

// Return the html representation of emphasis text.
func (n *EmphasisNode) Render() string {
	var s string
	for _, node := range n.Nodes {
		s += node.Render()
	}
	return wrap(n.Tag(), s)
}

func (p *parse) newEmphasis(pos Pos, style itemType) *EmphasisNode {
	return &EmphasisNode{NodeType: NodeEmphasis, Pos: pos, Style: style}
}

// HeadingNode holds heaing element with specific level(1-6).
type HeadingNode struct {
	NodeType
	Pos
	Level int
	Text  string
	Nodes []Node
}

// Render returns the html representation based on heading level.
func (n *HeadingNode) Render() (s string) {
	for _, node := range n.Nodes {
		s += node.Render()
	}
	re := regexp.MustCompile(`[^\w]+`)
	id := re.ReplaceAllString(n.Text, "-")
	// ToLowerCase
	id = strings.ToLower(id)
	return fmt.Sprintf("<%[1]s id=\"%s\">%s</%[1]s>", "h"+strconv.Itoa(n.Level), id, s)
}

func (p *parse) newHeading(pos Pos, level int, text string) *HeadingNode {
	return &HeadingNode{NodeType: NodeHeading, Pos: pos, Level: level, Text: p.text(text)}
}

// Code holds CodeBlock node with specific lang field.
type CodeNode struct {
	NodeType
	Pos
	Lang, Text string
}

// Return the html representation of codeBlock
func (n *CodeNode) Render() string {
	var attr string
	if n.Lang != "" {
		attr = fmt.Sprintf(" class=\"lang-%s\"", n.Lang)
	}
	code := fmt.Sprintf("<%[1]s%s>%s</%[1]s>", "code", attr, n.Text)
	return wrap("pre", code)
}

func (p *parse) newCode(pos Pos, lang, text string) *CodeNode {
	// DRY: see `escape()` below
	text = strings.NewReplacer("<", "&lt;", ">", "&gt;", "\"", "&quot;", "&", "&amp;").Replace(text)
	return &CodeNode{NodeType: NodeCode, Pos: pos, Lang: lang, Text: text}
}

// Link holds a tag with optional title
type LinkNode struct {
	NodeType
	Pos
	Title, Href string
	Nodes       []Node
}

// Return the html representation of link node
func (n *LinkNode) Render() (s string) {
	for _, node := range n.Nodes {
		s += node.Render()
	}
	attrs := fmt.Sprintf("href=\"%s\"", n.Href)
	if n.Title != "" {
		attrs += fmt.Sprintf(" title=\"%s\"", n.Title)
	}
	return fmt.Sprintf("<a %s>%s</a>", attrs, s)
}

func (p *parse) newLink(pos Pos, title, href string, nodes ...Node) *LinkNode {
	return &LinkNode{NodeType: NodeLink, Pos: pos, Title: p.text(title), Href: p.text(href), Nodes: nodes}
}

// RefLink holds link with refrence to link definition
type RefNode struct {
	NodeType
	Pos
	tr             *parse
	Text, Ref, Raw string
	Nodes          []Node
}

// rendering based type
func (n *RefNode) Render() string {
	var node Node
	ref := strings.ToLower(n.Ref)
	if l, ok := n.tr.links[ref]; ok {
		if n.Type() == NodeRefLink {
			node = n.tr.newLink(n.Pos, l.Title, l.Href, n.Nodes...)
		} else {
			node = n.tr.newImage(n.Pos, l.Title, l.Href, n.Text)
		}
	} else {
		node = n.tr.newText(n.Pos, n.Raw)
	}
	return node.Render()
}

// newRefLink create new RefLink that suitable for link
func (p *parse) newRefLink(typ itemType, pos Pos, raw, ref string, text []Node) *RefNode {
	return &RefNode{NodeType: NodeRefLink, Pos: pos, tr: p.root(), Raw: raw, Ref: ref, Nodes: text}
}

// newRefImage create new RefLink that suitable for image
func (p *parse) newRefImage(typ itemType, pos Pos, raw, ref, text string) *RefNode {
	return &RefNode{NodeType: NodeRefImage, Pos: pos, tr: p.root(), Raw: raw, Ref: ref, Text: text}
}

// DefLinkNode refresent single reference to link-definition
type DefLinkNode struct {
	NodeType
	Pos
	Name, Href, Title string
}

// Deflink have no representation(Transparent node)
func (n *DefLinkNode) Render() string {
	return ""
}

func (p *parse) newDefLink(pos Pos, name, href, title string) *DefLinkNode {
	return &DefLinkNode{NodeType: NodeLink, Pos: pos, Name: name, Href: href, Title: title}
}

// ImageNode represents an image element with optional alt and title attributes.
type ImageNode struct {
	NodeType
	Pos
	Title, Src, Alt string
}

// Render returns the html representation on image node
func (n *ImageNode) Render() string {
	attrs := fmt.Sprintf("src=\"%s\" alt=\"%s\"", n.Src, n.Alt)
	if n.Title != "" {
		attrs += fmt.Sprintf(" title=\"%s\"", n.Title)
	}
	return fmt.Sprintf("<img %s>", attrs)
}

func (p *parse) newImage(pos Pos, title, src, alt string) *ImageNode {
	return &ImageNode{NodeType: NodeImage, Pos: pos, Title: p.text(title), Src: p.text(src), Alt: p.text(alt)}
}

// ListNode holds list items nodes in ordered or unordered states.
type ListNode struct {
	NodeType
	Pos
	Ordered bool
	Items   []*ListItemNode
}

func (n *ListNode) append(item *ListItemNode) {
	n.Items = append(n.Items, item)
}

// Render returns the html representation of orderd(ol) or unordered(ul) list.
func (n *ListNode) Render() (s string) {
	tag := "ul"
	if n.Ordered {
		tag = "ol"
	}
	for _, item := range n.Items {
		s += "\n" + item.Render()
	}
	s += "\n"
	return wrap(tag, s)
}

func (p *parse) newList(pos Pos, ordered bool) *ListNode {
	return &ListNode{NodeType: NodeList, Pos: pos, Ordered: ordered}
}

// ListItem represents single item in ListNode that may contains nested nodes.
type ListItemNode struct {
	NodeType
	Pos
	Nodes []Node
}

func (l *ListItemNode) append(n Node) {
	l.Nodes = append(l.Nodes, n)
}

// Render returns the html representation of list-item
func (l *ListItemNode) Render() (s string) {
	for _, node := range l.Nodes {
		s += node.Render()
	}
	return wrap("li", s)
}

func (p *parse) newListItem(pos Pos) *ListItemNode {
	return &ListItemNode{NodeType: NodeListItem, Pos: pos}
}

// TableNode represents table element contains head and body
type TableNode struct {
	NodeType
	Pos
	Rows []*RowNode
}

func (n *TableNode) append(row *RowNode) {
	n.Rows = append(n.Rows, row)
}

// Render returns the html representation of a table
func (n *TableNode) Render() string {
	var s string
	for i, row := range n.Rows {
		s += "\n"
		switch i {
		case 0:
			s += wrap("thead", "\n"+row.Render()+"\n")
		case 1:
			s += "<tbody>\n"
			fallthrough
		default:
			s += row.Render()
		}
	}
	s += "\n</tbody>\n"
	return wrap("table", s)
}

func (p *parse) newTable(pos Pos) *TableNode {
	return &TableNode{NodeType: NodeTable, Pos: pos}
}

// RowNode represnt tr that holds list of cell-nodes
type RowNode struct {
	NodeType
	Pos
	Cells []*CellNode
}

func (r *RowNode) append(cell *CellNode) {
	r.Cells = append(r.Cells, cell)
}

// Render returns the html representation of table-row
func (r *RowNode) Render() string {
	var s string
	for _, cell := range r.Cells {
		s += "\n" + cell.Render()
	}
	s += "\n"
	return wrap("tr", s)
}

func (p *parse) newRow(pos Pos) *RowNode {
	return &RowNode{NodeType: NodeRow, Pos: pos}
}

// AlignType identifies the aligment-type of specfic cell.
type AlignType int

// Align returns itself and provides an easy default implementation
// for embedding in a Node.
func (t AlignType) Align() AlignType {
	return t
}

// Alignment
const (
	None AlignType = iota
	Right
	Left
	Center
)

// Cell types
const (
	Header = iota
	Data
)

// CellNode represents table-data/cell that holds simple text(may be emphasis)
// Note: the text in <th> elements are bold and centered by default.
type CellNode struct {
	NodeType
	Pos
	AlignType
	Kind  int
	Nodes []Node
}

// Render returns the html reprenestation of table-cell
func (c *CellNode) Render() string {
	var s string
	tag := "td"
	if c.Kind == Header {
		tag = "th"
	}
	for _, node := range c.Nodes {
		s += node.Render()
	}
	return fmt.Sprintf("<%[1]s%s>%s</%[1]s>", tag, c.Style(), s)
}

// Style return the cell-style based on alignment field
func (c *CellNode) Style() string {
	s := " style=\"text-align:"
	switch c.Align() {
	case Right:
		s += "right\""
	case Left:
		s += "left\""
	case Center:
		s += "center\""
	default:
		s = ""
	}
	return s
}

func (p *parse) newCell(pos Pos, kind int, align AlignType) *CellNode {
	return &CellNode{NodeType: NodeCell, Pos: pos, Kind: kind, AlignType: align}
}

// BlockQuote represents block-quote tag.
type BlockQuoteNode struct {
	NodeType
	Pos
	Nodes []Node
}

// Render returns the html representation of BlockQuote
func (n *BlockQuoteNode) Render() string {
	var s string
	for _, node := range n.Nodes {
		s += node.Render()
	}
	return wrap("blockquote", s)
}

func (p *parse) newBlockQuote(pos Pos) *BlockQuoteNode {
	return &BlockQuoteNode{NodeType: NodeBlockQuote, Pos: pos}
}

// CheckboxNode represents checked and unchecked checkbox tag.
// Used in task lists.
type CheckboxNode struct {
	NodeType
	Pos
	Checked bool
}

// Render returns the html representation of checked and unchecked CheckBox.
func (n *CheckboxNode) Render() string {
	s := "<input type=\"checkbox\""
	if n.Checked {
		s += " checked"
	}
	return s + ">"
}

func (p *parse) newCheckbox(pos Pos, checked bool) *CheckboxNode {
	return &CheckboxNode{NodeType: NodeCheckbox, Pos: pos, Checked: checked}
}

// Wrap text with specific tag.
func wrap(tag, body string) string {
	return fmt.Sprintf("<%[1]s>%s</%[1]s>", tag, body)
}

// Group all text configuration in one place(escaping, smartypants, etc..)
func (p *parse) text(input string) string {
	opts := p.root().options
	if opts.Smartypants {
		input = smartypants(input)
	}
	if opts.Fractions {
		input = smartyfractions(input)
	}
	return escape(input)
}

// Helper escaper
func escape(str string) (cpy string) {
	emp := regexp.MustCompile(`&\w+;`)
	for i := 0; i < len(str); i++ {
		switch s := str[i]; s {
		case '>':
			cpy += "&gt;"
		case '"':
			cpy += "&quot;"
		case '\'':
			cpy += "&#39;"
		case '<':
			if res := reHTML.tag.FindString(str[i:]); res != "" {
				cpy += res
				i += len(res) - 1
			} else {
				cpy += "&lt;"
			}
		case '&':
			if res := emp.FindString(str[i:]); res != "" {
				cpy += res
				i += len(res) - 1
			} else {
				cpy += "&amp;"
			}
		default:
			cpy += str[i : i+1]
		}
	}
	return
}

// Smartypants transformation helper, translate from marked.js
func smartypants(text string) string {
	// em-dashes, en-dashes, ellipses
	re := strings.NewReplacer("---", "\u2014", "--", "\u2013", "...", "\u2026")
	text = re.Replace(text)
	// opening singles
	text = regexp.MustCompile("(^|[-\u2014/(\\[{\"\\s])'").ReplaceAllString(text, "$1\u2018")
	// closing singles & apostrophes
	text = strings.Replace(text, "'", "\u2019", -1)
	// opening doubles
	text = regexp.MustCompile("(^|[-\u2014/(\\[{\u2018\\s])\"").ReplaceAllString(text, "$1\u201c")
	// closing doubles
	text = strings.Replace(text, "\"", "\u201d", -1)
	return text
}

// Smartyfractions transformation helper.
func smartyfractions(text string) string {
	re := regexp.MustCompile(`(\d+)(/\d+)(/\d+|)`)
	return re.ReplaceAllStringFunc(text, func(str string) string {
		var match []string
		// If it's date like
		if match = re.FindStringSubmatch(str); match[3] != "" {
			return str
		}
		switch n := match[1] + match[2]; n {
		case "1/2", "1/3", "2/3", "1/4", "3/4", "1/5", "2/5", "3/5", "4/5",
			"1/6", "5/6", "1/7", "1/8", "3/8", "5/8", "7/8":
			return fmt.Sprintf("&frac%s;", strings.Replace(n, "/", "", 1))
		default:
			return fmt.Sprintf("<sup>%s</sup>&frasl;<sub>%s</sub>",
				match[1], strings.Replace(match[2], "/", "", 1))
		}
	})
}
