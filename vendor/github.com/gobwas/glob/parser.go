package glob

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

type node interface {
	children() []node
	append(node)
}

// todo may be split it into another package
type lexerIface interface {
	nextItem() item
}

type nodeImpl struct {
	desc []node
}

func (n *nodeImpl) append(c node) {
	n.desc = append(n.desc, c)
}
func (n *nodeImpl) children() []node {
	return n.desc
}

type nodeList struct {
	nodeImpl
	not   bool
	chars string
}
type nodeRange struct {
	nodeImpl
	not    bool
	lo, hi rune
}
type nodeText struct {
	nodeImpl
	text string
}

type nodePattern struct{ nodeImpl }
type nodeAny struct{ nodeImpl }
type nodeSuper struct{ nodeImpl }
type nodeSingle struct{ nodeImpl }
type nodeAnyOf struct{ nodeImpl }

type tree struct {
	root    node
	current node
	path    []node
}

func (t *tree) enter(c node) {
	if t.root == nil {
		t.root = c
		t.current = c
		return
	}

	t.current.append(c)
	t.path = append(t.path, c)
	t.current = c
}

func (t *tree) leave() {
	if len(t.path)-1 <= 0 {
		t.current = t.root
		t.path = nil
		return
	}

	t.path = t.path[:len(t.path)-1]
	t.current = t.path[len(t.path)-1]
}

type parseFn func(*tree, lexerIface) (parseFn, error)

func parse(lexer lexerIface) (*nodePattern, error) {
	var parser parseFn

	root := &nodePattern{}
	tree := &tree{}
	tree.enter(root)

	for parser = parserMain; ; {
		next, err := parser(tree, lexer)
		if err != nil {
			return nil, err
		}

		if next == nil {
			break
		}

		parser = next
	}

	return root, nil
}

func parserMain(tree *tree, lexer lexerIface) (parseFn, error) {
	for stop := false; !stop; {
		item := lexer.nextItem()

		switch item.t {
		case item_eof:
			stop = true
			continue

		case item_error:
			return nil, errors.New(item.s)

		case item_text:
			tree.current.append(&nodeText{text: item.s})
			return parserMain, nil

		case item_any:
			tree.current.append(&nodeAny{})
			return parserMain, nil

		case item_super:
			tree.current.append(&nodeSuper{})
			return parserMain, nil

		case item_single:
			tree.current.append(&nodeSingle{})
			return parserMain, nil

		case item_range_open:
			return parserRange, nil

		case item_terms_open:
			tree.enter(&nodeAnyOf{})
			tree.enter(&nodePattern{})
			return parserMain, nil

		case item_separator:
			tree.leave()
			tree.enter(&nodePattern{})
			return parserMain, nil

		case item_terms_close:
			tree.leave()
			tree.leave()
			return parserMain, nil

		default:
			return nil, fmt.Errorf("unexpected token: %s", item)
		}
	}

	return nil, nil
}

func parserRange(tree *tree, lexer lexerIface) (parseFn, error) {
	var (
		not   bool
		lo    rune
		hi    rune
		chars string
	)

	for {
		item := lexer.nextItem()

		switch item.t {
		case item_eof:
			return nil, errors.New("unexpected end")

		case item_error:
			return nil, errors.New(item.s)

		case item_not:
			not = true

		case item_range_lo:
			r, w := utf8.DecodeRuneInString(item.s)
			if len(item.s) > w {
				return nil, fmt.Errorf("unexpected length of lo character")
			}

			lo = r

		case item_range_between:
			//

		case item_range_hi:
			r, w := utf8.DecodeRuneInString(item.s)
			if len(item.s) > w {
				return nil, fmt.Errorf("unexpected length of lo character")
			}

			hi = r

			if hi < lo {
				return nil, fmt.Errorf("hi character '%s' should be greater than lo '%s'", string(hi), string(lo))
			}

		case item_text:
			chars = item.s

		case item_range_close:
			isRange := lo != 0 && hi != 0
			isChars := chars != ""

			if isChars == isRange {
				return nil, fmt.Errorf("could not parse range")
			}

			if isRange {
				tree.current.append(&nodeRange{
					lo:  lo,
					hi:  hi,
					not: not,
				})
			} else {
				tree.current.append(&nodeList{
					chars: chars,
					not:   not,
				})
			}

			return parserMain, nil
		}
	}
}
