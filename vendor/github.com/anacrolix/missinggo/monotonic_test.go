package missinggo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Calls suite with the used time.Now function used by MonotonicNow replaced
// with stdNow for the duration of the call.
func withCustomStdNow(stdNow func() time.Time, suite func()) {
	oldStdNow := stdNowFunc
	oldSkew := monotonicSkew
	defer func() {
		stdNowFunc = oldStdNow
		monotonicSkew = oldSkew
	}()
	stdNowFunc = stdNow
	suite()
}

// Returns a time.Now-like function that walks seq returning time.Unix(0,
// seq[i]) in successive calls.
func stdNowSeqFunc(seq []int64) func() time.Time {
	var i int
	return func() time.Time {
		defer func() { i++ }()
		return time.Unix(0, seq[i])
	}
}

func TestMonotonicTime(t *testing.T) {
	started := MonotonicNow()
	withCustomStdNow(stdNowSeqFunc([]int64{2, 1, 3, 3, 2, 3}), func() {
		i0 := MonotonicNow() // 0
		i1 := MonotonicNow() // 1
		assert.EqualValues(t, 0, i0.Sub(i1))
		assert.EqualValues(t, 2, MonotonicSince(i0)) // 2
		assert.EqualValues(t, 2, MonotonicSince(i1)) // 3
		i4 := MonotonicNow()
		assert.EqualValues(t, 2, i4.Sub(i0))
		assert.EqualValues(t, 2, i4.Sub(i1))
		i5 := MonotonicNow()
		assert.EqualValues(t, 3, i5.Sub(i0))
		assert.EqualValues(t, 3, i5.Sub(i1))
		assert.EqualValues(t, 1, i5.Sub(i4))
	})
	// Ensure that skew and time function are restored correctly and within
	// reasonable bounds.
	assert.True(t, MonotonicSince(started) >= 0 && MonotonicSince(started) < time.Second)
}
