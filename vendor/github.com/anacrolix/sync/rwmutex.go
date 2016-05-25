package sync

import "sync"

// This RWMutex's RLock and RUnlock methods don't allow shared reading because
// there's no way to determine what goroutine has stopped holding the read
// lock when RUnlock is called. So for debugging purposes, it's just like
// Mutex.
type RWMutex struct {
	ins Mutex        // Instrumented
	rw  sync.RWMutex // Real McCoy
}

func (me *RWMutex) Lock() {
	if enabled {
		me.ins.Lock()
	} else {
		me.rw.Lock()
	}
}

func (me *RWMutex) Unlock() {
	if enabled {
		me.ins.Unlock()
	} else {
		me.rw.Unlock()
	}
}

func (me *RWMutex) RLock() {
	if enabled {
		me.ins.Lock()
	} else {
		me.rw.RLock()
	}
}
func (me *RWMutex) RUnlock() {
	if enabled {
		me.ins.Unlock()
	} else {
		me.rw.RUnlock()
	}
}
