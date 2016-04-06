package lite

type Graph []Node

type Node struct {
	Edges []Edge
}
type Edge struct {
	Start, End int
	S, E       interface{}
}

func NewGraph(size int) Graph {
	if size < 0 {
		return Graph(make([]Node, 0))
	}
	return Graph(make([]Node, size))
}
