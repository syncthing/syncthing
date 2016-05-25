package sync

import (
	"bytes"
	"sync"
	"testing"
)

func init() {
	Enable()
}

func TestLog(t *testing.T) {
	var mu Mutex
	mu.Lock()
	mu.Unlock()
}

func TestRWMutex(t *testing.T) {
	var mu RWMutex
	mu.RLock()
	mu.RUnlock()
}

func TestUnlockUnlocked(t *testing.T) {
	var mu sync.Mutex
	defer func() {
		err := recover()
		if err == nil {
			t.Fatal("should have panicked")
		}
	}()
	mu.Unlock()
}

func TestPointerCompare(t *testing.T) {
	a, b := new(int), new(int)
	if a == b {
		t.FailNow()
	}
}

func TestLockTime(t *testing.T) {
	var buf bytes.Buffer
	PrintLockTimes(&buf)
	t.Log(buf.String())
}
