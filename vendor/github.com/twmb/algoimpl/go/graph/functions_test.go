package graph

import (
	"testing"
)

// RandMinimumCut has been tested in practice (Coursera Algo course 1). If any bugs crop up, email me.

func BenchmarkTopologicalSort(b *testing.B) {
	b.StopTimer()
	graph, _ := setupTopologicalSort()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		graph.TopologicalSort()
	}
}

func TestTopologicalSort(t *testing.T) {
	graph, wantOrder := setupTopologicalSort()
	result := graph.TopologicalSort()
	firstLen := len(result)
	result = graph.TopologicalSort()
	if len(result) != firstLen {
		t.Errorf("topologicalSort 2 times fails")
	}
	for i := range result {
		if result[i] != wantOrder[i] {
			t.Errorf("index %v in result != wanted, value: %v, want value: %v", i, result[i], wantOrder[i])
		}
	}
}

func setupTopologicalSort() (*Graph, []Node) {
	graph := New(Directed)
	nodes := make([]Node, 0)
	// create graph on page 613 of CLRS ed. 3
	nodes = append(nodes, graph.MakeNode()) // shirt
	nodes = append(nodes, graph.MakeNode()) // tie
	nodes = append(nodes, graph.MakeNode()) // jacket
	nodes = append(nodes, graph.MakeNode()) // belt
	nodes = append(nodes, graph.MakeNode()) // watch
	nodes = append(nodes, graph.MakeNode()) // undershorts
	nodes = append(nodes, graph.MakeNode()) // pants
	nodes = append(nodes, graph.MakeNode()) // shoes
	nodes = append(nodes, graph.MakeNode()) // socks
	graph.MakeEdge(nodes[0], nodes[1])
	graph.MakeEdge(nodes[1], nodes[2])
	graph.MakeEdge(nodes[0], nodes[3])
	graph.MakeEdge(nodes[3], nodes[2])
	graph.MakeEdge(nodes[5], nodes[6])
	graph.MakeEdge(nodes[5], nodes[7])
	graph.MakeEdge(nodes[6], nodes[3])
	graph.MakeEdge(nodes[6], nodes[7])
	graph.MakeEdge(nodes[8], nodes[7])
	wantOrder := make([]Node, len(graph.nodes))
	wantOrder[0] = nodes[8] // socks
	wantOrder[1] = nodes[5] // undershorts
	wantOrder[2] = nodes[6] // pants
	wantOrder[3] = nodes[7] // shoes
	wantOrder[4] = nodes[4] // watch
	wantOrder[5] = nodes[0] // shirt
	wantOrder[6] = nodes[3] // belt
	wantOrder[7] = nodes[1] // tie
	wantOrder[8] = nodes[2] // jacket
	return graph, wantOrder
}

func TestStronglyConnectedComponents(t *testing.T) {
	testSCCDirected(t)
	testSCCUndirected(t)
}

func testSCCDirected(t *testing.T) {
	graph, want := setupSCCDirected()
	components := graph.StronglyConnectedComponents()
	for j := range components {
		for i := range want[j] {
			if !componentContains(components[j], want[j][i]) {
				t.Errorf("component slice %v does not contain want node %v", components[j], want[j][i])
			}
		}
	}
}

func testSCCUndirected(t *testing.T) {
	g, want := setupSCCUndirected()
	components := g.StronglyConnectedComponents()
	for j := range components {
		for i := range want[j] {
			if !componentContains(components[j], want[j][i]) {
				t.Errorf("component slice %v does not contain want node %v", components[j], want[j][i])
			}
		}
	}
}

func BenchmarkSCCUndirected(b *testing.B) {
	b.StopTimer()
	graph, _ := setupSCCUndirected()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		graph.StronglyConnectedComponents()
	}
}

func BenchmarkSCCDirected(b *testing.B) {
	b.StopTimer()
	graph, _ := setupSCCDirected()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		graph.StronglyConnectedComponents()
	}
}

func setupSCCDirected() (*Graph, [][]Node) {
	graph := New(Directed)
	nodes := make([]Node, 0)
	// create SCC graph on page 616 of CLRS ed 3
	nodes = append(nodes, graph.MakeNode()) //0, c
	nodes = append(nodes, graph.MakeNode()) //1, g
	nodes = append(nodes, graph.MakeNode()) //2, f
	nodes = append(nodes, graph.MakeNode()) //3, h
	nodes = append(nodes, graph.MakeNode()) //4, d
	nodes = append(nodes, graph.MakeNode()) //5, b
	nodes = append(nodes, graph.MakeNode()) //6, e
	nodes = append(nodes, graph.MakeNode()) //7, a
	graph.MakeEdge(nodes[0], nodes[1])
	graph.MakeEdge(nodes[0], nodes[4])
	graph.MakeEdge(nodes[1], nodes[2])
	graph.MakeEdge(nodes[1], nodes[3])
	graph.MakeEdge(nodes[2], nodes[1])
	graph.MakeEdge(nodes[3], nodes[3])
	graph.MakeEdge(nodes[4], nodes[3])
	graph.MakeEdge(nodes[4], nodes[0])
	graph.MakeEdge(nodes[5], nodes[6])
	graph.MakeEdge(nodes[5], nodes[0])
	graph.MakeEdge(nodes[5], nodes[2])
	graph.MakeEdge(nodes[6], nodes[2])
	graph.MakeEdge(nodes[6], nodes[7])
	graph.MakeEdge(nodes[7], nodes[5])
	want := make([][]Node, 4)
	want[0] = make([]Node, 3)
	want[1] = make([]Node, 2)
	want[2] = make([]Node, 2)
	want[3] = make([]Node, 1)
	want[0][0] = nodes[6]
	want[0][1] = nodes[7]
	want[0][2] = nodes[5]
	want[1][0] = nodes[0]
	want[1][1] = nodes[4]
	want[2][0] = nodes[2]
	want[2][1] = nodes[1]
	want[3][0] = nodes[3]
	return graph, want
}

func setupSCCUndirected() (*Graph, [][]Node) {
	g := New(Undirected)
	nodes := make([]Node, 0)
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	nodes = append(nodes, g.MakeNode())
	g.MakeEdge(nodes[0], nodes[1])
	g.MakeEdge(nodes[0], nodes[10])
	g.MakeEdge(nodes[2], nodes[3])  // 1: 0--1    2: 2--3
	g.MakeEdge(nodes[3], nodes[4])  //    | /           |
	g.MakeEdge(nodes[4], nodes[5])  //    10         5--4
	g.MakeEdge(nodes[8], nodes[7])  //          8
	g.MakeEdge(nodes[9], nodes[8])  //    3:   /|\
	g.MakeEdge(nodes[8], nodes[6])  //        / | \
	g.MakeEdge(nodes[10], nodes[1]) //       7  9  6
	want := make([][]Node, 3)
	want[0] = make([]Node, 3)
	want[1] = make([]Node, 4)
	want[2] = make([]Node, 4)
	want[0][0] = nodes[0]
	want[0][1] = nodes[1]
	want[0][2] = nodes[10]
	want[1][0] = nodes[2]
	want[1][1] = nodes[3]
	want[1][2] = nodes[4]
	want[1][3] = nodes[5]
	want[2][0] = nodes[8]
	want[2][1] = nodes[7]
	want[2][2] = nodes[9]
	want[2][3] = nodes[6]
	return g, want
}

func TestMinimumSpanningTree(t *testing.T) {
	g := New(Undirected)
	nodes := make(map[string]Node, 0)
	nodes["a"] = g.MakeNode()
	nodes["b"] = g.MakeNode()
	nodes["c"] = g.MakeNode()
	nodes["d"] = g.MakeNode()
	nodes["e"] = g.MakeNode()
	nodes["f"] = g.MakeNode()
	nodes["g"] = g.MakeNode()
	nodes["h"] = g.MakeNode()
	nodes["i"] = g.MakeNode()
	for key, node := range nodes {
		*node.Value = key
	}
	g.MakeEdgeWeight(nodes["a"], nodes["b"], 4)
	g.MakeEdgeWeight(nodes["a"], nodes["h"], 8)
	g.MakeEdgeWeight(nodes["b"], nodes["h"], 11)
	g.MakeEdgeWeight(nodes["b"], nodes["c"], 8)
	g.MakeEdgeWeight(nodes["c"], nodes["i"], 2)
	g.MakeEdgeWeight(nodes["c"], nodes["f"], 4)
	g.MakeEdgeWeight(nodes["c"], nodes["d"], 7)
	g.MakeEdgeWeight(nodes["d"], nodes["e"], 9)
	g.MakeEdgeWeight(nodes["d"], nodes["f"], 14)
	g.MakeEdgeWeight(nodes["e"], nodes["f"], 10)
	g.MakeEdgeWeight(nodes["f"], nodes["g"], 2)
	g.MakeEdgeWeight(nodes["g"], nodes["h"], 1)
	g.MakeEdgeWeight(nodes["g"], nodes["i"], 6)
	g.MakeEdgeWeight(nodes["h"], nodes["i"], 7)
	mst := g.MinimumSpanningTree()
	mstNodes := make(map[Node]bool, 0)
	spanCost := 0
	for _, edge := range mst {
		if _, exists := mstNodes[edge.Start]; !exists {
			mstNodes[edge.Start] = true
		}
		if _, exists := mstNodes[edge.End]; !exists {
			mstNodes[edge.End] = true
		}
		spanCost += edge.Weight
	}
	if len(mstNodes) != len(nodes) { // 9
		t.Errorf("mst: # of nodes in MST is %v, expected %v", len(mstNodes), len(nodes))
	}
	if spanCost != 37 {
		t.Errorf("mst: expected MST cost of 37, got %v", spanCost)
	}

}

func componentContains(component []Node, node Node) bool {
	for i := range component {
		if component[i].node.index == node.node.index { // for SCC, the nodes will be reversed but the indices will be the same
			return true
		}
	}
	return false
}
