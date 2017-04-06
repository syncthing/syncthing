package deadlock

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/petermattis/goid"
)

// Opts control how deadlock detection behaves.
// Options are supposed to be set once at a startup (say, when parsing flags).
var Opts = struct {
	// Mutex/RWMutex would work exactly as their sync counterparts
	// -- almost no runtime penalty, no deadlock detection if Disable == true.
	Disable bool
	// Would disable lock order based deadlock detection if DisableLockOrderDetection == true.
	DisableLockOrderDetection bool
	// Waiting for a lock for longer than DeadlockTimeout is considered a deadlock.
	// Ignored is DeadlockTimeout <= 0.
	DeadlockTimeout time.Duration
	// OnPotentialDeadlock is called each time a potential deadlock is deetcted -- either based on
	// lock order or on lock wait time.
	OnPotentialDeadlock func()
	// Will keep MaxMapSize lock pairs (happens before // happens after) in the map.
	// The map resets once the threshold is reached.
	MaxMapSize int
	// Will print to deadlock info to log buffer.
	LogBuf io.Writer
}{
	DeadlockTimeout: time.Second * 30,
	OnPotentialDeadlock: func() {
		os.Exit(2)
	},
	MaxMapSize: 1024 * 64,
	LogBuf:     os.Stderr,
}

// A Mutex is a drop-in replacement for sync.Mutex.
// Performs deadlock detection unless disabled in Opts.
type Mutex struct {
	mu sync.Mutex
}

// Lock locks the mutex.
// If the lock is already in use, the calling goroutine
// blocks until the mutex is available.
//
// Unless deadlock detection is disabled, logs potential deadlocks to stderr,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *Mutex) Lock() {
	lock(m.mu.Lock, m)
}

// Unlock unlocks the mutex.
// It is a run-time error if m is not locked on entry to Unlock.
//
// A locked Mutex is not associated with a particular goroutine.
// It is allowed for one goroutine to lock a Mutex and then
// arrange for another goroutine to unlock it.
func (m *Mutex) Unlock() {
	m.mu.Unlock()
	if !Opts.Disable {
		PostUnlock(m)
	}
}

// An RWMutex is a drop-in replacement for sync.RWMutex.
// Performs deadlock detection unless disabled in Opts.
type RWMutex struct {
	mu sync.RWMutex
}

// Lock locks rw for writing.
// If the lock is already locked for reading or writing,
// Lock blocks until the lock is available.
// To ensure that the lock eventually becomes available,
// a blocked Lock call excludes new readers from acquiring
// the lock.
//
// Unless deadlock detection is disabled, logs potential deadlocks to stderr,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *RWMutex) Lock() {
	lock(m.mu.Lock, m)
}

// Unlock unlocks the mutex for writing.  It is a run-time error if rw is
// not locked for writing on entry to Unlock.
//
// As with Mutexes, a locked RWMutex is not associated with a particular
// goroutine.  One goroutine may RLock (Lock) an RWMutex and then
// arrange for another goroutine to RUnlock (Unlock) it.
func (m *RWMutex) Unlock() {
	m.mu.Unlock()
	if !Opts.Disable {
		PostUnlock(m)
	}
}

// RLock locks the mutex for reading.
//
// Unless deadlock detection is disabled, logs potential deadlocks to stderr,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *RWMutex) RLock() {
	lock(m.mu.RLock, m)
}

// RUnlock undoes a single RLock call;
// it does not affect other simultaneous readers.
// It is a run-time error if rw is not locked for reading
// on entry to RUnlock.
func (m *RWMutex) RUnlock() {
	if !Opts.Disable {
		PostUnlock(m)
	}
	m.mu.RUnlock()
}

// RLocker returns a Locker interface that implements
// the Lock and Unlock methods by calling RLock and RUnlock.
func (m *RWMutex) RLocker() sync.Locker {
	return (*rlocker)(m)
}

func PreLock(skip int, p interface{}) {
	lo.PreLock(skip, p)
}

func PostLock(skip int, p interface{}) {
	lo.PostLock(skip, p)
}

func PostUnlock(p interface{}) {
	lo.PostUnlock(p)
}

func lock(lockFn func(), ptr interface{}) {
	if Opts.Disable {
		lockFn()
		return
	}
	PreLock(4, ptr)
	if Opts.DeadlockTimeout <= 0 {
		lockFn()
	} else {
		ch := make(chan struct{})
		go func() {
			lockFn()
			close(ch)
		}()
		for {
			t := time.NewTimer(Opts.DeadlockTimeout)
			defer t.Stop()
			select {
			case <-t.C:
				lo.mu.Lock()
				prev, ok := lo.cur[ptr]
				if !ok {
					lo.mu.Unlock()
					break // Nobody seems to be holding a lock, try again.
				}
				fmt.Fprintln(Opts.LogBuf, header)
				fmt.Fprintln(Opts.LogBuf, "Previous place where the lock was grabbed")
				fmt.Fprintf(Opts.LogBuf, "goroutine %v lock %p\n", prev.gid, ptr)
				printStack(Opts.LogBuf, prev.stack)
				fmt.Fprintln(Opts.LogBuf, "Have been trying to lock it again for more than", Opts.DeadlockTimeout)
				fmt.Fprintf(Opts.LogBuf, "goroutine %v lock %p\n", goid.Get(), ptr)
				printStack(Opts.LogBuf, callers(2))
				fmt.Fprintln(Opts.LogBuf)
				stacks := stacks()
				grs := bytes.Split(stacks, []byte("\n\n"))
				for _, g := range grs {
					if goid.ExtractGID(g) == prev.gid {
						fmt.Fprintln(Opts.LogBuf, "Here is what goroutine", prev.gid, "doing now")
						Opts.LogBuf.Write(g)
						fmt.Fprintln(Opts.LogBuf)
					}
				}
				lo.other(ptr)
				fmt.Fprintln(Opts.LogBuf, "All current goroutines:")
				Opts.LogBuf.Write(stacks)
				lo.mu.Unlock()
				Opts.OnPotentialDeadlock()
				<-ch
				PostLock(4, ptr)
				return
			case <-ch:
				PostLock(4, ptr)
				return
			}
		}
	}
	PostLock(4, ptr)
}

type lockOrder struct {
	mu    sync.Mutex
	cur   map[interface{}]stackGID // stacktraces + gids for the locks currently taken.
	order map[beforeAfter]ss       // expected order of locks.
}

type stackGID struct {
	stack []uintptr
	gid   int64
}

type beforeAfter struct {
	before interface{}
	after  interface{}
}

type ss struct {
	before []uintptr
	after  []uintptr
}

var lo = newLockOrder()

func newLockOrder() *lockOrder {
	return &lockOrder{
		cur:   map[interface{}]stackGID{},
		order: map[beforeAfter]ss{},
	}
}

func (l *lockOrder) PostLock(skip int, p interface{}) {
	stack := callers(skip)
	gid := goid.Get()
	l.mu.Lock()
	l.cur[p] = stackGID{stack, gid}
	l.mu.Unlock()
}

func (l *lockOrder) PreLock(skip int, p interface{}) {
	if Opts.DisableLockOrderDetection {
		return
	}
	stack := callers(skip)
	gid := goid.Get()
	l.mu.Lock()
	for b, bs := range l.cur {
		if b == p {
			continue
		}
		if bs.gid != gid { // We want locks taken in the same goroutine only.
			continue
		}
		if s, ok := l.order[beforeAfter{p, b}]; ok {
			fmt.Fprintln(Opts.LogBuf, header, "Inconsistent locking. saw this ordering in one goroutine:")
			fmt.Fprintln(Opts.LogBuf, "happened before")
			printStack(Opts.LogBuf, s.before)
			fmt.Fprintln(Opts.LogBuf, "happened after")
			printStack(Opts.LogBuf, s.after)
			fmt.Fprintln(Opts.LogBuf, "in another goroutine: happened before")
			printStack(Opts.LogBuf, bs.stack)
			fmt.Fprintln(Opts.LogBuf, "happend after")
			printStack(Opts.LogBuf, stack)
			l.other(p)
			Opts.OnPotentialDeadlock()
		}
		l.order[beforeAfter{b, p}] = ss{bs.stack, stack}
		if len(l.order) == Opts.MaxMapSize { // Reset the map to keep memory footprint bounded.
			l.order = map[beforeAfter]ss{}
		}
	}
	l.cur[p] = stackGID{stack, gid}
	l.mu.Unlock()
}

func (l *lockOrder) PostUnlock(p interface{}) {
	l.mu.Lock()
	delete(l.cur, p)
	l.mu.Unlock()
}

type rlocker RWMutex

func (r *rlocker) Lock()   { (*RWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*RWMutex)(r).RUnlock() }

// Under lo.mu Locked.
func (l *lockOrder) other(ptr interface{}) {
	fmt.Fprintln(Opts.LogBuf, "\nOther goroutines holding locks:")
	for k, pp := range l.cur {
		if k == ptr {
			continue
		}
		fmt.Fprintf(Opts.LogBuf, "goroutine %v lock %p\n", pp.gid, k)
		printStack(Opts.LogBuf, pp.stack)
	}
	fmt.Fprintln(Opts.LogBuf)
}

const header = "POTENTIAL DEADLOCK:"
