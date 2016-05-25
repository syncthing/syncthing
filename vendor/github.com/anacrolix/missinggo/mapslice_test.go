package missinggo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapSlice(t *testing.T) {
	sl := MapAsSlice(map[string]int{"two": 2, "one": 1})
	assert.Len(t, sl, 2)
	assert.EqualValues(t, []MapKeyValue{{"one", 1}, {"two", 2}}, Sort(sl, func(left, right MapKeyValue) bool {
		return left.Key.(string) < right.Key.(string)
	}))
}
