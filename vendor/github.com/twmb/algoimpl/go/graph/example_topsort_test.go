package graph_test

import (
	"fmt"
	"github.com/twmb/algoimpl/go/graph"
)

func ExampleGraph_TopologicalSort() {
	g := graph.New(graph.Directed)

	clothes := make(map[string]graph.Node, 0)
	// Make a mapping from strings to a node
	clothes["shirt"] = g.MakeNode()
	clothes["tie"] = g.MakeNode()
	clothes["jacket"] = g.MakeNode()
	clothes["belt"] = g.MakeNode()
	clothes["watch"] = g.MakeNode()
	clothes["undershorts"] = g.MakeNode()
	clothes["pants"] = g.MakeNode()
	clothes["shoes"] = g.MakeNode()
	clothes["socks"] = g.MakeNode()
	// Make references back to the string values
	for key, node := range clothes {
		*node.Value = key
	}
	// Connect the elements
	g.MakeEdge(clothes["shirt"], clothes["tie"])
	g.MakeEdge(clothes["tie"], clothes["jacket"])
	g.MakeEdge(clothes["shirt"], clothes["belt"])
	g.MakeEdge(clothes["belt"], clothes["jacket"])
	g.MakeEdge(clothes["undershorts"], clothes["pants"])
	g.MakeEdge(clothes["undershorts"], clothes["shoes"])
	g.MakeEdge(clothes["pants"], clothes["belt"])
	g.MakeEdge(clothes["pants"], clothes["shoes"])
	g.MakeEdge(clothes["socks"], clothes["shoes"])
	sorted := g.TopologicalSort()
	for i := range sorted {
		fmt.Println(*sorted[i].Value)
	}
	// Output:
	// socks
	// undershorts
	// pants
	// shoes
	// watch
	// shirt
	// belt
	// tie
	// jacket
}
