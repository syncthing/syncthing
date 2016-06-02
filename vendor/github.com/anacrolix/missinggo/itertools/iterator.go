package itertools

import "github.com/anacrolix/missinggo"

type Iterator interface {
	// Advances to the next value. Returns false if there are no more values.
	// Must be called before the first value.
	Next() bool
	// Returns the current value. Should panic when the iterator is in an
	// invalid state.
	Value() interface{}
	// Ceases iteration prematurely. This should occur implicitly if Next
	// returns false.
	Stop()
}

type sliceIterator struct {
	slice []interface{}
	value interface{}
	ok    bool
}

func (me *sliceIterator) Next() bool {
	if len(me.slice) == 0 {
		return false
	}
	me.value = me.slice[0]
	me.slice = me.slice[1:]
	me.ok = true
	return true
}

func (me *sliceIterator) Value() interface{} {
	if !me.ok {
		panic("no value; call Next")
	}
	return me.value
}

func (me *sliceIterator) Stop() {}

func SliceIterator(a []interface{}) Iterator {
	return &sliceIterator{
		slice: a,
	}
}

func StringIterator(a string) Iterator {
	return SliceIterator(missinggo.ConvertToSliceOfEmptyInterface(a))
}

func IteratorAsSlice(it Iterator) (ret []interface{}) {
	for it.Next() {
		ret = append(ret, it.Value())
	}
	return
}
