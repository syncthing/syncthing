package missinggo

import (
	"math"
	"time"
)

// Returns a time.Timer that calls f. The timer is initially stopped.
func StoppedFuncTimer(f func()) (t *time.Timer) {
	t = time.AfterFunc(math.MaxInt64, f)
	if !t.Stop() {
		panic("timer already fired")
	}
	return
}
