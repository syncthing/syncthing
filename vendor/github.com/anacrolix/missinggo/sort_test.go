package missinggo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSort(t *testing.T) {
	a := []int{3, 2, 1}
	Sort(a, func(left, right int) bool {
		return left < right
	})
	assert.EqualValues(t, []int{1, 2, 3}, a)
}
