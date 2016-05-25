package bitmap

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/itertools"
)

func TestEmptyBitmap(t *testing.T) {
	var bm Bitmap
	assert.False(t, bm.Contains(0))
	bm.Remove(0)
	it := itertools.NewIterator(&bm)
	assert.Panics(t, func() { it.Value() })
	assert.False(t, it.Next())
}

func bitmapSlice(bm *Bitmap) (ret []int) {
	sl := itertools.IterableAsSlice(bm)
	missinggo.CastSlice(&ret, sl)
	return
}

func TestSimpleBitmap(t *testing.T) {
	bm := new(Bitmap)
	assert.EqualValues(t, []int(nil), bitmapSlice(bm))
	bm.Add(0)
	assert.True(t, bm.Contains(0))
	assert.False(t, bm.Contains(1))
	assert.EqualValues(t, 1, bm.Len())
	bm.Add(3)
	assert.True(t, bm.Contains(0))
	assert.True(t, bm.Contains(3))
	assert.EqualValues(t, []int{0, 3}, bitmapSlice(bm))
	assert.EqualValues(t, 2, bm.Len())
	bm.Remove(0)
	assert.EqualValues(t, []int{3}, bitmapSlice(bm))
	assert.EqualValues(t, 1, bm.Len())
}

func TestSub(t *testing.T) {
	var left, right Bitmap
	left.Add(2, 5, 4)
	right.Add(3, 2, 6)
	assert.Equal(t, []int{4, 5}, Sub(&left, &right).ToSortedSlice())
	assert.Equal(t, []int{3, 6}, Sub(&right, &left).ToSortedSlice())
}

func TestSubUninited(t *testing.T) {
	var left, right Bitmap
	assert.EqualValues(t, []int(nil), Sub(&left, &right).ToSortedSlice())
}

func TestAddRange(t *testing.T) {
	var bm Bitmap
	bm.AddRange(21, 26)
	bm.AddRange(9, 14)
	bm.AddRange(11, 16)
	bm.Remove(12)
	assert.EqualValues(t, []int{9, 10, 11, 13, 14, 15, 21, 22, 23, 24, 25}, bm.ToSortedSlice())
	assert.EqualValues(t, 11, bm.Len())
	bm.Clear()
	bm.AddRange(3, 7)
	bm.AddRange(0, 3)
	bm.AddRange(2, 4)
	bm.Remove(3)
	assert.EqualValues(t, []int{0, 1, 2, 4, 5, 6}, bm.ToSortedSlice())
	assert.EqualValues(t, 6, bm.Len())
}

func TestRemoveRange(t *testing.T) {
	var bm Bitmap
	bm.AddRange(3, 12)
	assert.EqualValues(t, 9, bm.Len())
	bm.RemoveRange(14, -1)
	assert.EqualValues(t, 9, bm.Len())
	bm.RemoveRange(2, 5)
	assert.EqualValues(t, 7, bm.Len())
	bm.RemoveRange(10, -1)
	assert.EqualValues(t, 5, bm.Len())
}

func TestLimits(t *testing.T) {
	var bm Bitmap
	assert.Panics(t, func() { bm.Add(math.MaxInt64) })
	bm.Add(-1)
	assert.EqualValues(t, 1, bm.Len())
	assert.EqualValues(t, []int{MaxInt}, bm.ToSortedSlice())
}
