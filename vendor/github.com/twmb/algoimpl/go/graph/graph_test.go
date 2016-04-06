package graph

import (
	"testing"
)

func (g *Graph) edgeBack(from, to *node) bool {
	for _, v := range from.edges {
		if v.end == to {
			return true
		}
	}
	return false
}

func (g *Graph) reversedEdgeBack(from, to *node) bool {
	for _, v := range to.reversedEdges {
		if v.end == from {
			return true
		}
	}
	return false
}

func nodeSliceContains(slice []Node, node Node) bool {
	for i := range slice {
		if slice[i].node == node.node { // used for exact node checking as opposed to functions_test componentContains
			return true
		}
	}
	return false
}

func (g *Graph) verify(t *testing.T) {
	// over all the nodes
	for i, node := range g.nodes {
		if node.index != i {
			t.Errorf("node's graph index %v != actual graph index %v", node.index, i)
		}
		// over each edge
		for _, edge := range node.edges {

			// check that the graph contains it in the correct position
			if edge.end.index >= len(g.nodes) {
				t.Errorf("adjacent node end graph index %v >= len(g.nodes)%v", edge.end.index, len(g.nodes))
			}
			if g.nodes[edge.end.index] != edge.end {
				t.Errorf("adjacent node %p does not belong to the graph on edge %v: should be %p", edge.end, edge, g.nodes[edge.end.index])
			}
			// if graph is undirected, check that the to node's reversed edges connects to the from edge
			if g.Kind == Directed {
				if !g.reversedEdgeBack(node, edge.end) {
					t.Errorf("directed graph: node %v has edge to %v, reversedEdges start at end does not have edge back to node", node, edge.end)
				}
			}
			// if the graph is undirected, check that the adjacent node contains the original node back
			if g.Kind == Undirected {
				if !g.edgeBack(edge.end, node) {
					t.Errorf("undirected graph: node %v has adjacent node %v, adjacent node doesn't contain back", node, edge.end)
				}
			}
		}
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		InKind   GraphType
		WantKind GraphType
	}{
		{Directed, 1},
		{Undirected, 0},
		{3, 0},
	}

	for _, test := range tests {
		got := New(test.InKind)
		if got.Kind != test.WantKind {
			t.Errorf("Received wrong kind of graph")
		}
		if len(got.nodes) > 0 {
			t.Errorf("Received new graph has nodes %v, shouldn't", got.nodes)
		}
	}
}

func TestMakeNode(t *testing.T) {
	graph := New(Undirected)
	nodes := make(map[Node]int, 0)
	for i := 0; i < 10; i++ {
		nodes[graph.MakeNode()] = i
	}
	graph.verify(t)
	graph = New(Directed)
	nodes = make(map[Node]int, 0)
	for i := 0; i < 10; i++ {
		nodes[graph.MakeNode()] = i
	}
	graph.verify(t)
}

func TestRemoveNode(t *testing.T) {
	g := New(Undirected)
	nodes := make([]Node, 2)
	nodes[0] = g.MakeNode()
	nodes[1] = g.MakeNode()
	g.MakeEdge(nodes[0], nodes[0])
	g.MakeEdge(nodes[1], nodes[0])
	g.MakeEdge(nodes[0], nodes[1])
	g.MakeEdge(nodes[1], nodes[1])
	g.verify(t)
	g.RemoveNode(&nodes[1])
	g.verify(t)
	g.RemoveNode(&nodes[1])
	g.verify(t)
	g.RemoveNode(&nodes[0])
	g.verify(t)
	nodes = make([]Node, 10)
	g = New(Directed)
	for i := 0; i < 10; i++ {
		nodes[i] = g.MakeNode()
	}
	// connect every node to every node
	for j := 0; j < 10; j++ {
		for i := 0; i < 10; i++ {
			if g.MakeEdge(nodes[i], nodes[j]) != nil {
				t.Errorf("could not connect %v, %v", i, j)
			}
		}
	}
	g.verify(t)
	g.RemoveNode(&nodes[0])
	g.verify(t)
	if nodes[0].node != nil {
		t.Errorf("Node still has reference to node in graph")
	}
	g.RemoveNode(&nodes[9])
	g.verify(t)
	g.RemoveNode(&nodes[9])
	g.verify(t)
	g.RemoveNode(&nodes[0])
	g.verify(t)
	g.RemoveNode(&nodes[1])
	g.verify(t)
	g.RemoveNode(&nodes[2])
	g.verify(t)
	g.RemoveNode(&nodes[3])
	g.verify(t)
	g.RemoveNode(&nodes[4])
	g.verify(t)
	g.RemoveNode(&nodes[5])
	g.verify(t)
	g.RemoveNode(&nodes[6])
	g.verify(t)
	g.RemoveNode(&nodes[7])
	g.verify(t)
	g.RemoveNode(&nodes[8])
	g.verify(t)
	g.RemoveNode(&nodes[9])
	g.verify(t)
}

func TestMakeEdge(t *testing.T) {
	graph := New(Undirected)
	mapped := make(map[int]Node, 0)
	for i := 0; i < 10; i++ {
		mapped[i] = graph.MakeNode()
	}
	for j := 0; j < 5; j++ {
		for i := 0; i < 10; i++ {
			graph.MakeEdge(mapped[i], mapped[(i+1+j)%10])
		}
	}
	graph.verify(t)
	for i, node := range graph.nodes {
		if mapped[i].node != node {
			t.Errorf("Node at index %v != %v, wrong!", i, i)
		}
	}
	var nonGraphNode Node
	err := graph.MakeEdge(nonGraphNode, mapped[0])
	if err == nil {
		t.Errorf("err was nil when expecting error for connecting non graph node to graph")
	}
	graph = New(Directed)
	mapped = make(map[int]Node, 0)
	for i := 0; i < 10; i++ {
		mapped[i] = graph.MakeNode()
	}
	for j := 0; j < 5; j++ {
		for i := 0; i < 10; i++ {
			graph.MakeEdge(mapped[i], mapped[(i+1+j)%10])
		}
	}
	graph.verify(t)
	for i, node := range graph.nodes {
		if mapped[i].node != node {
			t.Errorf("Node at index %v = %v, != %v, wrong!", i, mapped[i], node)
		}
	}
}

func TestRemoveEdge(t *testing.T) {
	g := New(Undirected)
	nodes := make([]Node, 2)
	nodes[0] = g.MakeNode()
	nodes[1] = g.MakeNode()
	g.MakeEdge(nodes[0], nodes[0])
	g.MakeEdge(nodes[1], nodes[0])
	g.MakeEdge(nodes[0], nodes[1])
	g.MakeEdge(nodes[1], nodes[1])
	g.verify(t)
	g.RemoveEdge(nodes[0], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[0], nodes[1])
	g.verify(t)
	g.RemoveEdge(nodes[1], nodes[1])
	g.verify(t)

	nodes = make([]Node, 10)
	g = New(Directed)
	for i := 0; i < 10; i++ {
		nodes[i] = g.MakeNode()
	}
	// connect every node to every node
	for j := 0; j < 10; j++ {
		for i := 0; i < 10; i++ {
			if g.MakeEdge(nodes[i], nodes[j]) != nil {
				t.Errorf("could not connect %v, %v", i, j)
			}
		}
	}
	g.verify(t)
	g.RemoveEdge(nodes[5], nodes[4])
	g.verify(t)
	g.RemoveEdge(nodes[9], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[9], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[0], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[1], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[2], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[3], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[4], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[5], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[6], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[7], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[8], nodes[0])
	g.verify(t)
	g.RemoveEdge(nodes[9], nodes[0])
	g.verify(t)
}

func TestNeighbors(t *testing.T) {
	g := New(Undirected)
	nodes := make([]Node, 2)
	nodes[0] = g.MakeNode()
	nodes[1] = g.MakeNode()
	g.MakeEdge(nodes[1], nodes[0])
	g.verify(t)
	neighbors := g.Neighbors(nodes[0])
	if !nodeSliceContains(neighbors, nodes[1]) {
		t.Errorf("nodes 1 not a neighbor of 0, even though connected in undirected graph")
	}
	oldGraphNode := nodes[0]
	nodes = make([]Node, 3)
	g = New(Directed)
	nodes[0] = g.MakeNode()
	nodes[1] = g.MakeNode()
	nodes[2] = g.MakeNode()
	g.MakeEdge(nodes[1], nodes[0])
	g.MakeEdge(nodes[2], nodes[1]) // 2->1->0
	g.verify(t)
	neighbors = g.Neighbors(nodes[1])
	if nodeSliceContains(neighbors, nodes[2]) {
		t.Errorf("nodes 2 is a neighbor of 0, even though not 1 not connected to 2 in undirected graph")
	}
	if !nodeSliceContains(neighbors, nodes[0]) {
		t.Errorf("nodes 0 not a neighbor of 0, even though 1 connects to 0 in directed graph")
	}
	neighbors = g.Neighbors(oldGraphNode)
	if len(neighbors) > 0 {
		t.Errorf("old graph node has neighbors in new graph: %v", neighbors)
	}
}
