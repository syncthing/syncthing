package missinggo

import (
	"sync"
	"time"
)

// Monotonic time represents time since an arbitrary point in the past, where
// the concept of now is only ever moving in a positive direction.
type MonotonicTime struct {
	skewedStdTime time.Time
}

func (me MonotonicTime) Sub(other MonotonicTime) time.Duration {
	return me.skewedStdTime.Sub(other.skewedStdTime)
}

var (
	stdNowFunc    = time.Now
	monotonicMu   sync.Mutex
	lastStdNow    time.Time
	monotonicSkew time.Duration
)

func skewedStdNow() time.Time {
	monotonicMu.Lock()
	defer monotonicMu.Unlock()
	stdNow := stdNowFunc()
	if !lastStdNow.IsZero() && stdNow.Before(lastStdNow) {
		monotonicSkew += lastStdNow.Sub(stdNow)
	}
	lastStdNow = stdNow
	return stdNow.Add(monotonicSkew)
}

// Consecutive calls always produce the same or greater time than previous
// calls.
func MonotonicNow() MonotonicTime {
	return MonotonicTime{skewedStdNow()}
}

func MonotonicSince(since MonotonicTime) (ret time.Duration) {
	return skewedStdNow().Sub(since.skewedStdTime)
}
