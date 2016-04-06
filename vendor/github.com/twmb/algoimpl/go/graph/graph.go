// Implements an adjacency list graph as a slice of generic nodes
// and includes some useful graph functions.
package graph

import (
	"errors"
)

// Directed or undirected.
type GraphType int

const (
	Undirected GraphType = iota
	Directed
)

// Graph is an adjacency slice representation of a graph. Can be directed or undirected.
type Graph struct {
	nodes []*node
	Kind  GraphType
}

type node struct {
	edges         []edge
	reversedEdges []edge
	index         int
	state         int   // used for metadata
	data          int   // also used for metadata
	parent        *node // also used for metadata
	container     Node  // who holds me
}

// Node connects to a backing node on the graph. It can safely be used in maps.
type Node struct {
	// In an effort to prevent access to the actual graph
	// and so that the Node type can be used in a map while
	// the graph changes metadata, the Node type encapsulates
	// a pointer to the actual node data.
	node *node
	// Value can be used to store information on the caller side.
	// Its use is optional. See the Topological Sort example for
	// a reason on why to use this pointer.
	// The reason it is a pointer is so that graph function calls
	// can test for equality on Nodes. The pointer wont change,
	// the value it points to will. If the pointer is explicitly changed,
	// graph functions that use Nodes will cease to work.
	Value *interface{}
}

type edge struct {
	weight int
	end    *node
}

// An Edge connects two Nodes in a graph. To modify Weight, use
// the MakeEdgeWeight function. Any local modifications will
// not be seen in the graph.
type Edge struct {
	Weight int
	Start  Node
	End    Node
}

// New creates and returns an empty graph.
// If kind is Directed, returns a directed graph.
// This function returns an undirected graph by default.
func New(kind GraphType) *Graph {
	g := &Graph{}
	if kind == Directed {
		g.Kind = Directed
	}
	return g
}

// MakeNode creates a node, adds it to the graph and returns the new node.
func (g *Graph) MakeNode() Node {
	newNode := &node{index: len(g.nodes)}
	newNode.container = Node{node: newNode, Value: new(interface{})}
	g.nodes = append(g.nodes, newNode)
	return newNode.container
}

// RemoveNode removes a node from the graph and all edges connected to it.
// This function nils points in the Node structure. If 'remove' is used in
// a map, you must delete the map index first.
func (g *Graph) RemoveNode(remove *Node) {
	if remove.node == nil {
		return
	}
	// O(V)
	nodeExists := false
	// remove all edges that connect from a different node to this one
	for _, node := range g.nodes {
		if node == remove.node {
			nodeExists = true
			continue
		}

		// O(E)
		swapIndex := -1 // index that the edge-to-remove is at
		for i := range node.edges {
			if node.edges[i].end == remove.node {
				swapIndex = i
			}
		}
		if swapIndex > -1 {
			swapNRemoveEdge(swapIndex, &node.edges)
		}

		// deal with possible reversed edge
		swapIndex = -1
		for i := range node.reversedEdges {
			if node.reversedEdges[i].end == remove.node {
				swapIndex = i
			}
		}
		if swapIndex > -1 {
			swapNRemoveEdge(swapIndex, &node.reversedEdges)
		}
		if node.index > remove.node.index {
			node.index--
		}
	}
	if nodeExists {
		copy(g.nodes[remove.node.index:], g.nodes[remove.node.index+1:])
		g.nodes = g.nodes[:len(g.nodes)-1]
	}
	remove.node.parent = nil
	remove.node = nil
}

// MakeEdge calls MakeEdgeWeight with a weight of 0 and returns an error if either of the nodes do not
// belong in the graph. Calling MakeEdge multiple times on the same nodes will not create multiple edges.
func (g *Graph) MakeEdge(from, to Node) error {
	return g.MakeEdgeWeight(from, to, 0)
}

// MakeEdgeWeight creates  an edge in the graph with a corresponding weight.
// It returns an error if either of the nodes do not belong in the graph.
//
// Calling MakeEdgeWeight multiple times on the same nodes will not create multiple edges;
// this function will update the weight on the node to the new value.
func (g *Graph) MakeEdgeWeight(from, to Node, weight int) error {
	if from.node == nil || from.node.index >= len(g.nodes) || g.nodes[from.node.index] != from.node {
		return errors.New("First node in MakeEdge call does not belong to this graph")
	}
	if to.node == nil || to.node.index >= len(g.nodes) || g.nodes[to.node.index] != to.node {
		return errors.New("Second node in MakeEdge call does not belong to this graph")
	}

	for i := range from.node.edges { // check if edge already exists
		if from.node.edges[i].end == to.node {
			from.node.edges[i].weight = weight

			// If the graph is undirected, fix the to node's weight as well
			if g.Kind == Undirected && to != from {
				for j := range to.node.edges {
					if to.node.edges[j].end == from.node {
						to.node.edges[j].weight = weight
					}
				}
			}
			return nil
		}
	}
	newEdge := edge{weight: weight, end: to.node}
	from.node.edges = append(from.node.edges, newEdge)
	reversedEdge := edge{weight: weight, end: from.node} // weight for undirected graph only
	if g.Kind == Directed {                              // reversed edges are only used in directed graph algorithms
		to.node.reversedEdges = append(to.node.reversedEdges, reversedEdge)
	}
	if g.Kind == Undirected && to != from {
		to.node.edges = append(to.node.edges, reversedEdge)
	}
	return nil
}

// RemoveEdge removes edges starting at the from node and ending at the to node.
// If the graph is undirected, RemoveEdge will remove all edges between the nodes.
func (g *Graph) RemoveEdge(from, to Node) {
	fromEdges := from.node.edges
	toEdges := to.node.edges
	toReversedEdges := to.node.reversedEdges
	for e := range fromEdges { // fix from->to
		if fromEdges[e].end == to.node {
			swapNRemoveEdge(e, &fromEdges)
			from.node.edges = fromEdges
			break
		}
	}
	for e := range toReversedEdges { // fix reversed edges to->from
		if toReversedEdges[e].end == from.node {
			swapNRemoveEdge(e, &toReversedEdges)
			to.node.reversedEdges = toReversedEdges
			break
		}
	}
	if g.Kind == Undirected && from.node != to.node {
		for e := range toEdges {
			if toEdges[e].end == from.node {
				swapNRemoveEdge(e, &toEdges)
				to.node.edges = toEdges
				break
			}
		}
	}
}

// Neighbors returns a slice of nodes that are reachable from the given node in a graph.
func (g *Graph) Neighbors(n Node) []Node {
	neighbors := make([]Node, 0, len(n.node.edges))
	if g.nodes[n.node.index] == n.node {
		for _, edge := range n.node.edges {
			neighbors = append(neighbors, edge.end.container)
		}
	}
	return neighbors
}

// Swaps an edge to the end of the edges slice and 'removes' it by reslicing.
func swapNRemoveEdge(remove int, edges *[]edge) {
	(*edges)[remove], (*edges)[len(*edges)-1] = (*edges)[len(*edges)-1], (*edges)[remove]
	*edges = (*edges)[:len(*edges)-1]
}
