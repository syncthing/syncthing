package graph_test

import (
	"fmt"
	"github.com/twmb/algoimpl/go/graph"
)

func ExampleGraph_MinimumSpanningTree() {
	g := graph.New(graph.Undirected)
	nodes := make(map[rune]graph.Node, 0)
	nodes['a'] = g.MakeNode()
	nodes['b'] = g.MakeNode()
	nodes['c'] = g.MakeNode()
	nodes['d'] = g.MakeNode()
	nodes['e'] = g.MakeNode()
	nodes['f'] = g.MakeNode()
	nodes['g'] = g.MakeNode()
	nodes['h'] = g.MakeNode()
	nodes['i'] = g.MakeNode()
	g.MakeEdgeWeight(nodes['a'], nodes['b'], 4)
	g.MakeEdgeWeight(nodes['a'], nodes['h'], 8)
	g.MakeEdgeWeight(nodes['b'], nodes['c'], 8)
	g.MakeEdgeWeight(nodes['b'], nodes['h'], 11)
	g.MakeEdgeWeight(nodes['c'], nodes['d'], 7)
	g.MakeEdgeWeight(nodes['c'], nodes['f'], 4)
	g.MakeEdgeWeight(nodes['c'], nodes['i'], 2)
	g.MakeEdgeWeight(nodes['d'], nodes['e'], 9)
	g.MakeEdgeWeight(nodes['d'], nodes['f'], 14)
	g.MakeEdgeWeight(nodes['e'], nodes['f'], 10)
	g.MakeEdgeWeight(nodes['f'], nodes['g'], 2)
	g.MakeEdgeWeight(nodes['g'], nodes['h'], 1)
	g.MakeEdgeWeight(nodes['g'], nodes['i'], 6)
	g.MakeEdgeWeight(nodes['h'], nodes['i'], 7)
	mst := g.MinimumSpanningTree()
	weightSum := 0
	for i := range mst {
		weightSum += mst[i].Weight
	}
	fmt.Println(weightSum)
	// Output: 37
}
