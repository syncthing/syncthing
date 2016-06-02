package missinggo

import (
	"math/rand"
	"time"
)

// Returns random duration in the range [average-plusMinus,
// average+plusMinus]. Negative plusMinus will likely panic. Be aware that if
// plusMinus >= average, you may get a zero or negative Duration. The
// distribution function is unspecified, in case I find a more appropriate one
// in the future.
func JitterDuration(average, plusMinus time.Duration) (ret time.Duration) {
	ret = average - plusMinus
	ret += time.Duration(rand.Int63n(2*int64(plusMinus) + 1))
	return
}
