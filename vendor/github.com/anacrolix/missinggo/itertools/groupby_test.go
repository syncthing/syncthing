package itertools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/missinggo"
)

func TestGroupByKey(t *testing.T) {
	var ks []byte
	gb := GroupBy(StringIterator("AAAABBBCCDAABBB"), nil)
	for gb.Next() {
		ks = append(ks, gb.Value().(Group).Key().(byte))
	}
	t.Log(ks)
	require.EqualValues(t, "ABCDAB", ks)
}

func TestGroupByList(t *testing.T) {
	var gs []string
	gb := GroupBy(StringIterator("AAAABBBCCD"), nil)
	for gb.Next() {
		i := gb.Value().(Iterator)
		var g string
		for i.Next() {
			g += string(i.Value().(byte))
		}
		gs = append(gs, g)
	}
	t.Log(gs)
}

func TestGroupByNiladicKey(t *testing.T) {
	const s = "AAAABBBCCD"
	gb := GroupBy(StringIterator(s), func(interface{}) interface{} { return nil })
	gb.Next()
	var ss []byte
	g := IteratorAsSlice(gb.Value().(Iterator))
	missinggo.CastSlice(&ss, g)
	assert.Equal(t, s, string(ss))
}

func TestNilEqualsNil(t *testing.T) {
	assert.False(t, nil == uniqueKey)
}
