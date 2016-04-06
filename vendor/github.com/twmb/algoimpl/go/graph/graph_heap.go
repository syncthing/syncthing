package graph

// Not that I'm falling prey to NIH syndrome, but implementing my own
// heap will speed up MST and Search algorithms because...
//
// Although asserting is relatively cheap, I also need update methods
// and can save time with an explicit update function.
// For reference, with one set of data:
//  - using containers/heap took 425 microseconds,
//  - switching to my own version of heap took 330 microseconds average,
//  - adding an update function takes 255 microseconds average.
// So this was worth it.

type nodeSlice []*node

func (n nodeSlice) less(i, j int) bool {
	return n[i].state < n[j].state
}
func (n nodeSlice) swap(i, j int) {
	n[j].data, n[i].data = n[i].data, n[j].data // swap data containing indices
	n[j], n[i] = n[i], n[j]
}

func (n nodeSlice) shuffleUp(index int) {
	for {
		parent := (index - 1) / 2
		if parent == index || n.less(parent, index) {
			break
		}
		n.swap(parent, index)
		index = parent
	}
}

func (n nodeSlice) shuffleDown(elem, to int) {
	for {
		minchild := elem*2 + 1 // left child: elem * 2 + 1
		if minchild >= to {
			return
		}
		rchild := minchild + 1 // left child + 1
		if rchild < to && n.less(rchild, minchild) {
			minchild = rchild
		}
		if !n.less(minchild, elem) {
			return
		}
		n.swap(minchild, elem)
		elem = minchild
	}
}

func (n nodeSlice) heapInit() {
	length := len(n)
	for i := length/2 - 1; i >= 0; i-- {
		n.shuffleDown(i, length)
	}
}

func (p *nodeSlice) remove(index int) *node {
	n := *p
	length := len(n) - 1
	if length != index {
		n.swap(length, index)
		n.shuffleDown(index, length)
		n.shuffleUp(index)
	}
	popped := n[length]
	popped.data = dequeued
	n = n[0:length]
	*p = n
	return popped
}

// I don't need shuffleDown call because all updates update to a
// smaller state (for now)
func (n nodeSlice) update(index, newState int) {
	n[index].state = newState
	n.shuffleUp(index)
}

func (p *nodeSlice) push(x *node) {
	n := *p
	x.data = len(n) // index into heap
	n = append(n, x)
	n.shuffleUp(len(n) - 1)
	*p = n
}

func (p *nodeSlice) pop() *node {
	return p.remove(0)
}
func (n nodeSlice) heapContains(node *node) bool { // extend heap interface
	return node.data > dequeued
}
