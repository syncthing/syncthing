package perf

import (
	"bytes"
	"expvar"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
)

var (
	em = missinggo.NewExpvarIndentMap("perfBuckets")
	mu sync.RWMutex
)

type Timer struct {
	started time.Time
}

func NewTimer() Timer {
	return Timer{time.Now()}
}

// The exponent is the upper bound of the duration in seconds.
func bucketExponent(d time.Duration) int {
	e := -9
	for d != 0 {
		d /= 10
		e++
	}
	return e
}

type buckets struct {
	mu      sync.Mutex
	buckets []int64
}

func (me *buckets) Add(t time.Duration) {
	e := bucketExponent(t)
	me.mu.Lock()
	for e+9 >= len(me.buckets) {
		me.buckets = append(me.buckets, 0)
	}
	me.buckets[e+9]++
	me.mu.Unlock()
}

func humanExponent(e int) string {
	if e == -9 {
		return "<1ns"
	}
	return ">" + time.Duration(math.Pow10(e-1)).String()
}

func (me *buckets) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{")
	first := true
	me.mu.Lock()
	for i, count := range me.buckets {
		if first {
			if count == 0 {
				continue
			}
			first = false
		} else {
			fmt.Fprintf(&b, ", ")
		}
		fmt.Fprintf(&b, "%q: %d", humanExponent(i), count)
	}
	me.mu.Unlock()
	fmt.Fprintf(&b, "}")
	return b.String()
}

var _ expvar.Var = &buckets{}

func (t *Timer) Stop(desc string) time.Duration {
	d := time.Since(t.started)
	mu.RLock()
	_m := em.Get(desc)
	mu.RUnlock()
	if _m == nil {
		mu.Lock()
		_m = em.Get(desc)
		if _m == nil {
			_m = new(buckets)
			em.Set(desc, _m)
		}
		mu.Unlock()
	}
	m := _m.(*buckets)
	m.Add(d)
	return d
}
