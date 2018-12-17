package ast

type Node struct {
	Parent   *Node
	Children []*Node
	Value    interface{}
	Kind     Kind
}

func NewNode(k Kind, v interface{}, ch ...*Node) *Node {
	n := &Node{
		Kind:  k,
		Value: v,
	}
	for _, c := range ch {
		Insert(n, c)
	}
	return n
}

func (a *Node) Equal(b *Node) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Value != b.Value {
		return false
	}
	if len(a.Children) != len(b.Children) {
		return false
	}
	for i, c := range a.Children {
		if !c.Equal(b.Children[i]) {
			return false
		}
	}
	return true
}

func Insert(parent *Node, children ...*Node) {
	parent.Children = append(parent.Children, children...)
	for _, ch := range children {
		ch.Parent = parent
	}
}

type List struct {
	Not   bool
	Chars string
}

type Range struct {
	Not    bool
	Lo, Hi rune
}

type Text struct {
	Text string
}

type Kind int

const (
	KindNothing Kind = iota
	KindPattern
	KindList
	KindRange
	KindText
	KindAny
	KindSuper
	KindSingle
	KindAnyOf
)
