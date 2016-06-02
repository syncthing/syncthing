package itertools

import "math/rand"

type seq struct {
	i []int
}

// Creates sequence of values from [0, n)
func newSeq(n int) seq {
	return seq{make([]int, n, n)}
}

func (me seq) Index(i int) (ret int) {
	ret = me.i[i]
	if ret == 0 {
		ret = i
	}
	return
}

func (me seq) Len() int {
	return len(me.i)
}

// Remove the nth value from the sequence.
func (me *seq) DeleteIndex(index int) {
	me.i[index] = me.Index(me.Len() - 1)
	me.i = me.i[:me.Len()-1]
}

func ForPerm(n int, callback func(i int) (more bool)) bool {
	s := newSeq(n)
	for s.Len() > 0 {
		r := rand.Intn(s.Len())
		if !callback(s.Index(r)) {
			return false
		}
		s.DeleteIndex(r)
	}
	return true
}
