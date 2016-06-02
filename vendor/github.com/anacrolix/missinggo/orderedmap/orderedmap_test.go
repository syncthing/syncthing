package orderedmap

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/missinggo/itertools"
)

func slice(om OrderedMap) (ret []interface{}) {
	om.Iter(func(i interface{}) bool {
		ret = append(ret, i)
		return true
	})
	return
}

func TestSimple(t *testing.T) {
	om := New(func(l, r interface{}) bool {
		return l.(int) < r.(int)
	})
	om.Set(3, 1)
	om.Set(2, 2)
	om.Set(1, 3)
	assert.EqualValues(t, []interface{}{3, 2, 1}, slice(om))
	om.Set(3, 2)
	om.Unset(2)
	assert.EqualValues(t, []interface{}{3, 2}, slice(om))
	om.Set(-1, 4)
	assert.EqualValues(t, []interface{}{4, 3, 2}, slice(om))
}

func TestIterEmpty(t *testing.T) {
	om := New(nil)
	it := itertools.NewIterator(om)
	assert.Panics(t, func() { it.Value() })
	assert.False(t, it.Next())
	it.Stop()
}
