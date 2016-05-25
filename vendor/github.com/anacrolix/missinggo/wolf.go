package missinggo

import (
	"log"
	"runtime"
	"sync"
	"sync/atomic"
)

const debug = false

// A Wolf represents some event that becomes less and less interesting as it
// occurs. Call CryHeard to see if we should pay attention this time.
type Wolf struct {
	cries uint64
}

// Returns true less and less often. Convenient for exponentially decreasing
// the amount of noise due to errors.
func (me *Wolf) CryHeard() bool {
	n := atomic.AddUint64(&me.cries, 1)
	return n&(n-1) == 0
}

var (
	mu     sync.Mutex
	wolves map[uintptr]*Wolf
)

// Calls CryHeard() on a Wolf that is unique to the callers program counter.
// i.e. every CryHeard() expression has its own Wolf.
func CryHeard() bool {
	pc, file, line, ok := runtime.Caller(1)
	if debug {
		log.Println(pc, file, line, ok)
	}
	if !ok {
		return true
	}
	mu.Lock()
	if wolves == nil {
		wolves = make(map[uintptr]*Wolf)
	}
	w, ok := wolves[pc]
	if !ok {
		w = new(Wolf)
		wolves[pc] = w
	}
	mu.Unlock()
	return w.CryHeard()
}
