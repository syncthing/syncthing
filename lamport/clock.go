package lamport

import "sync"

var Default = Clock{}

type Clock struct {
	val uint64
	mut sync.Mutex
}

func (c *Clock) Tick(v uint64) uint64 {
	c.mut.Lock()
	if v > c.val {
		c.val = v + 1
		c.mut.Unlock()
		return v + 1
	} else {
		c.val++
		v = c.val
		c.mut.Unlock()
		return v
	}
}
