// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"path/filepath"
	"sort"

	"github.com/twmb/algoimpl/go/graph"
)

func (c *ChangeSet) sortQueue() {
	// This is where the magic happens! We have various tricky cases that
	// need to be handled in terms of the order things must happen in. We
	// must delete children of a directory before deleting the directory
	// itself. This means deleting files before directories; except, if a
	// file is going to replace a directory, then we need to delete the files
	// in the directory, then the directory, then create the file, and so on.
	// This method declares the dependencies between the updates and performs
	// a topological sort to ensure depdencies are handled before their
	// dependents.

	// We go to some effort to create a graph that only contains the updates
	// that actually need an ordering. The rest of the updates are left in
	// their existing order, so that the user can choose if that's newest-
	// first or smallest-first and so on.

	// File name -> original queue position for updated and deleted files.
	// This is so we can look up files by name along the way. We have both a
	// delete and update for the same name, so we need two maps.
	updates := make(map[string]int)
	deletes := make(map[string]int)
	for i, f := range c.queue {
		if f.IsDeleted() {
			deletes[f.Name] = i
		} else {
			updates[f.Name] = i
		}
	}

	// File name -> graph node for updated and deleted files. Files are added
	// to these maps and to the graph when we discover that there is a
	// dependency between them.
	updateNodes := make(map[string]graph.Node)
	deleteNodes := make(map[string]graph.Node)
	g := graph.New(graph.Directed)

	// Create edges in the graph for dependencies. A call to g.MakeEdge(a, b)
	// creates an edge a->b to indicate that a must happen before b - that
	// is, b depends on a.

	for _, currentIdx := range updates {
		f := c.queue[currentIdx]

		// Updating an item x requires that we've already handled x's parent
		parentName := filepath.Dir(f.Name)
		if idx, ok := updates[parentName]; ok {
			parent := graphNode(g, updateNodes, c.queue[idx], idx)
			current := graphNode(g, updateNodes, f, currentIdx)
			g.MakeEdge(parent, current)
		}

		// If we have a delete for the same name as a file we're creating
		// (say, we're deleting a dir and creating a file there), the delete
		// must happen before the update.
		if delIdx, ok := deletes[f.Name]; ok {
			del := graphNode(g, deleteNodes, c.queue[delIdx], delIdx)
			current := graphNode(g, updateNodes, f, currentIdx)
			g.MakeEdge(del, current)
		}

		// If we have a delete for a file with identical contents to this
		// one, we want to make sure we do this update before that delete so
		// that we can reuse the blocks.
		if !f.IsDirectory() && !f.IsSymlink() {
			hash := string(f.Hash())
			if del, ok := c.deletedHashes[hash]; ok {
				delIdx := deletes[del.Name]
				del := graphNode(g, deleteNodes, c.queue[delIdx], delIdx)
				current := graphNode(g, updateNodes, f, currentIdx)
				g.MakeEdge(current, del)
			}
		}
	}

	for _, currentIdx := range deletes {
		f := c.queue[currentIdx]

		// Deleting an item x requires that we've already deleted x's
		// children. Finding the children is tricky, so we go through the
		// entire list and add dependencies from the parent if we find one.
		parentName := filepath.Dir(f.Name)
		if parentIdx, ok := deletes[parentName]; ok {
			parent := graphNode(g, deleteNodes, c.queue[parentIdx], parentIdx)
			current := graphNode(g, deleteNodes, f, currentIdx)
			g.MakeEdge(current, parent)
		}
	}

	// Sort the graph topologically to get a list of queue indexes in the
	// order they must be processed now.
	var order []int
	for _, node := range g.TopologicalSort() {
		order = append(order, (*node.Value).(int))
	}

	// To get the actual queue items into the order we require, we use
	// sort.Sort. The Swap(a, b) method of the queueSorter does the work, in
	// that it swaps both order[a]/order[b] and also
	// queue[order[a]]/queue[order[b]].
	ss := queueSorter{c.queue, order}
	sort.Sort(ss)

	// The end result is the queue with the items minimally rearranged to
	// fulfil the dependencies.
}

// graphNode returns an existing or new a graph.Node, inserted into the
// relevant map
func graphNode(g *graph.Graph, m map[string]graph.Node, f fileInfo, idx int) graph.Node {
	n, ok := m[f.Name]
	if !ok {
		n = g.MakeNode()
		*n.Value = idx
		m[f.Name] = n
	}
	return n
}

type queueSorter struct {
	queue []fileInfo
	order []int
}

func (s queueSorter) Len() int {
	return len(s.order)
}
func (s queueSorter) Less(a, b int) bool {
	return s.order[a] < s.order[b]
}
func (s queueSorter) Swap(a, b int) {
	s.queue[s.order[a]], s.queue[s.order[b]] = s.queue[s.order[b]], s.queue[s.order[a]]
	s.order[a], s.order[b] = s.order[b], s.order[a]
}
