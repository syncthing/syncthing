package semaphore

import (
	"context"
	"sync"
	"time"
)

// Semaphore is an implementation of semaphore.
type Semaphore struct {
	permits int
	avail   int
	channel chan struct{}
	aMutex  sync.Mutex   // acquire
	rMutex  sync.Mutex   // release
	pMutex  sync.RWMutex // number of permits
}

// New creates a new Semaphore with specified number of permits.
func New(permits int) *Semaphore {
	if permits < 1 {
		panic("Invalid number of permits. Less than 1")
	}

	// fill channel buffer
	channel := make(chan struct{}, permits)
	for i := 0; i < permits; i++ {
		channel <- struct{}{}
	}

	return &Semaphore{
		permits: permits,
		avail:   permits,
		channel: channel,
	}
}

// Acquire acquires one permit. If it is not available, the goroutine will block until it is available.
func (s *Semaphore) Acquire() {
	s.aMutex.Lock()
	defer s.aMutex.Unlock()

	s.pMutex.Lock()
	s.avail--
	s.pMutex.Unlock()

	<-s.channel
}

// AcquireMany is similar to Acquire() but for many permits.
//
// The number of permits acquired is at most the number of permits in the semaphore.
// i.e. if n = 5 and s was created with New(2), at most 2 permits will be acquired.
func (s *Semaphore) AcquireMany(n int) {
	if n > s.permits {
		n = s.permits
	}

	for ; n > 0; n-- {
		s.Acquire()
	}
}

// AcquireContext is similar to AcquireMany() but takes a context. Returns true if successful
// or false if the context is done first.
func (s *Semaphore) AcquireContext(ctx context.Context, n int) bool {
	acquired := make(chan struct{}, 1)
	reverse := make(chan bool, 1)
	go func() {
		s.AcquireMany(n)
		acquired <- struct{}{}
		if <-reverse {
			s.ReleaseMany(n)
		}
		close(acquired)
		close(reverse)
	}()

	select {
	case <-ctx.Done():
		reverse <- true
		return false
	case <-acquired:
		reverse <- false
		return true
	}
}

// AcquireWithin is similar to AcquireMany() but cancels if duration elapses before getting the permits.
// Returns true if successful and false if timeout occurs.
func (s *Semaphore) AcquireWithin(n int, d time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	return s.AcquireContext(ctx, n)
}

// Release releases one permit.
func (s *Semaphore) Release() {
	s.rMutex.Lock()
	defer s.rMutex.Unlock()

	s.channel <- struct{}{}

	s.pMutex.Lock()
	s.avail++
	s.pMutex.Unlock()
}

// ReleaseMany releases n permits.
//
// The number of permits released is at most the number of permits in the semaphore.
// i.e. if n = 5 and s was created with New(2), at most 2 permits will be released.
func (s *Semaphore) ReleaseMany(n int) {
	if n > s.permits {
		n = s.permits
	}

	for ; n > 0; n-- {
		s.Release()
	}
}

// AvailablePermits gives number of available unacquired permits.
func (s *Semaphore) AvailablePermits() int {
	s.pMutex.RLock()
	defer s.pMutex.RUnlock()

	if s.avail < 0 {
		return 0
	}
	return s.avail
}

// DrainPermits acquires all available permits and return the number of permits acquired.
func (s *Semaphore) DrainPermits() int {
	n := s.AvailablePermits()
	if n > 0 {
		s.AcquireMany(n)
	}
	return n
}
