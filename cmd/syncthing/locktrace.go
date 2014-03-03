//+build locktrace

package main

import (
	"log"
	"path"
	"runtime"
	"time"
)

var (
	lockTime time.Time
)

func (m *Model) Lock() {
	_, file, line, _ := runtime.Caller(1)
	log.Printf("%s:%d: Lock()...", path.Base(file), line)
	blockTime := time.Now()
	m.RWMutex.Lock()
	lockTime = time.Now()
	log.Printf("%s:%d: ...Lock() [%.04f ms]", path.Base(file), line, time.Since(blockTime).Seconds()*1000)
}

func (m *Model) Unlock() {
	_, file, line, _ := runtime.Caller(1)
	m.RWMutex.Unlock()
	log.Printf("%s:%d: Unlock() [%.04f ms]", path.Base(file), line, time.Since(lockTime).Seconds()*1000)
}

func (m *Model) RLock() {
	_, file, line, _ := runtime.Caller(1)
	log.Printf("%s:%d: RLock()...", path.Base(file), line)
	blockTime := time.Now()
	m.RWMutex.RLock()
	log.Printf("%s:%d: ...RLock() [%.04f ms]", path.Base(file), line, time.Since(blockTime).Seconds()*1000)
}

func (m *Model) RUnlock() {
	_, file, line, _ := runtime.Caller(1)
	m.RWMutex.RUnlock()
	log.Printf("%s:%d: RUnlock()", path.Base(file), line)
}
