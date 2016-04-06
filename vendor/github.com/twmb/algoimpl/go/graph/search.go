package graph

type Path struct {
	Weight int
	Path   []Edge
}

// DijkstraSearch returns the shortest path from the start node to every other
// node in the graph. All edges must have a positive weight, otherwise this
// function will return nil.
func (g *Graph) DijkstraSearch(start Node) []Path {
	if start.node == nil || g.nodes[start.node.index] != start.node {
		return nil
	}
	paths := make([]Path, len(g.nodes))

	nodesBase := nodeSlice(make([]*node, len(g.nodes)))
	copy(nodesBase, g.nodes)
	for i := range nodesBase {
		nodesBase[i].state = 1<<31 - 1
		nodesBase[i].data = i
	}
	start.node.state = 0 // make it so 'start' sorts to the top of the heap
	nodes := &nodesBase
	nodes.heapInit()

	for len(*nodes) > 0 {
		curNode := nodes.pop()
		for _, edge := range curNode.edges {
			newWeight := curNode.state + edge.weight
			if newWeight < curNode.state { // negative edge length
				return nil
			}
			v := edge.end
			if nodes.heapContains(v) && newWeight < v.state {
				v.parent = curNode
				nodes.update(v.data, newWeight)
			}
		}

		// build path to this node
		if curNode.parent != nil {
			newPath := Path{Weight: curNode.state}
			newPath.Path = make([]Edge, len(paths[curNode.parent.index].Path)+1)
			copy(newPath.Path, paths[curNode.parent.index].Path)
			newPath.Path[len(newPath.Path)-1] = Edge{Weight: curNode.state - curNode.parent.state,
				Start: curNode.parent.container, End: curNode.container}
			paths[curNode.index] = newPath
		} else {
			paths[curNode.index] = Path{Weight: curNode.state, Path: []Edge{}}
		}
	}
	return paths
}
