// Package sync records the stack when locks are taken, and when locks are
// blocked on and exports them as pprof profiles "lockHolders" and
// "lockBlockers". if "net/http/pprof" is imported, you can view them at
// /debug/pprof/ on the default HTTP muxer.
//
// The API mirrors that of stdlib "sync". The package can be imported in place
// of "sync", and is enabled by setting the envvar PPROF_SYNC non-empty.
//
// Note that currently RWMutex is treated like a Mutex when the package is
// enabled.
package sync

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/anacrolix/missinggo"
)

var (
	// Protects initialization and enabling of the package.
	enableMu sync.Mutex
	// Whether any of this package is to be active.
	enabled = false
	// Current lock holders.
	lockHolders *pprof.Profile
	// Those blocked on acquiring a lock.
	lockBlockers *pprof.Profile

	// Protects lockTimes.
	lockTimesMu sync.Mutex
	// The longest time the lock is held for any unique stack.
	lockTimes map[[32]uintptr]time.Duration
)

type lockTimesSorter struct {
	entries []lockTime
}

func (me *lockTimesSorter) Len() int { return len(me.entries) }
func (me *lockTimesSorter) Less(i, j int) bool {
	return me.entries[i].held < me.entries[j].held
}
func (me *lockTimesSorter) Swap(i, j int) {
	me.entries[i], me.entries[j] = me.entries[j], me.entries[i]
}

type lockTime struct {
	stack [32]uintptr
	held  time.Duration
}

func sortedLockTimes() []lockTime {
	var lts lockTimesSorter
	lockTimesMu.Lock()
	for stack, held := range lockTimes {
		lts.entries = append(lts.entries, lockTime{stack, held})
	}
	lockTimesMu.Unlock()
	sort.Sort(sort.Reverse(&lts))
	return lts.entries
}

func PrintLockTimes(w io.Writer) {
	lockTimes := sortedLockTimes()
	tw := tabwriter.NewWriter(w, 1, 8, 1, '\t', 0)
	defer tw.Flush()
	w = tw
	for _, elem := range lockTimes {
		fmt.Fprintf(w, "%s\n", elem.held)
		missinggo.WriteStack(w, elem.stack[:])
	}
}

func Enable() {
	enableMu.Lock()
	defer enableMu.Unlock()
	if enabled {
		return
	}
	lockTimes = make(map[[32]uintptr]time.Duration)
	lockHolders = pprof.NewProfile("lockHolders")
	lockBlockers = pprof.NewProfile("lockBlockers")
	http.DefaultServeMux.HandleFunc("/debug/lockTimes", func(w http.ResponseWriter, r *http.Request) {
		PrintLockTimes(w)
	})
	enabled = true
}

func init() {
	if os.Getenv("PPROF_SYNC") != "" {
		Enable()
	}
}

type Mutex struct {
	mu      sync.Mutex
	hold    *int        // Unique value for passing to pprof.
	stack   [32]uintptr // The stack for the current holder.
	start   time.Time   // When the lock was obtained.
	entries int         // Number of entries returned from runtime.Callers.
}

func (m *Mutex) Lock() {
	if !enabled {
		m.mu.Lock()
		return
	}
	v := new(int)
	lockBlockers.Add(v, 0)
	m.mu.Lock()
	lockBlockers.Remove(v)
	m.hold = v
	lockHolders.Add(v, 0)
	m.entries = runtime.Callers(2, m.stack[:])
	m.start = time.Now()
}
func (m *Mutex) Unlock() {
	if enabled {
		lockHeld := time.Since(m.start)
		var key [32]uintptr
		copy(key[:], m.stack[:m.entries])
		lockTimesMu.Lock()
		if lockHeld > lockTimes[key] {
			lockTimes[key] = lockHeld
		}
		lockTimesMu.Unlock()
		lockHolders.Remove(m.hold)
	}
	m.mu.Unlock()
}

type WaitGroup struct {
	sync.WaitGroup
}

type Cond struct {
	sync.Cond
}
