package examples

import (
	"fmt"

	"github.com/d4l3k/messagediff"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func ExampleAtom() {
	got := data2()
	want := data1()
	diff, equal := messagediff.PrettyDiff(want, got)
	fmt.Printf("%v %s", equal, diff)
	// Output: false modified: [0].FirstChild.NextSibling.Attr = " baz"
}

func data1() []*html.Node {
	n := &html.Node{
		Type: html.ElementNode, Data: atom.Span.String(),
		Attr: []html.Attribute{
			{Key: atom.Class.String(), Val: "foo"},
		},
	}
	n.AppendChild(
		&html.Node{
			Type: html.ElementNode, Data: atom.Span.String(),
			Attr: []html.Attribute{
				{Key: atom.Class.String(), Val: "bar"},
			},
		},
	)
	n.AppendChild(&html.Node{
		Type: html.TextNode, Data: "baz",
	})
	return []*html.Node{n}
}

func data2() []*html.Node {
	n := &html.Node{
		Type: html.ElementNode, Data: atom.Span.String(),
		Attr: []html.Attribute{
			{Key: atom.Class.String(), Val: "foo"},
		},
	}
	n.AppendChild(
		&html.Node{
			Type: html.ElementNode, Data: atom.Span.String(),
			Attr: []html.Attribute{
				{Key: atom.Class.String(), Val: "bar"},
			},
		},
	)
	n.AppendChild(&html.Node{
		Type: html.TextNode, Data: " baz",
	})
	return []*html.Node{n}
}
