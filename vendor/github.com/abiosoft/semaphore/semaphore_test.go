package semaphore

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

var done = make(chan int, 3)
var g = &sync.WaitGroup{}

func Test(t *testing.T) {
	l := 7
	s := New(10)
	for i := 0; i < l; i++ {
		g.Add(1)
		go aq(s, i+1)
	}
	go func() {
		g.Add(1)
		if s.AcquireWithin(5, time.Second*3) {
			s.ReleaseMany(5)
			fmt.Println("Acquired within")
		} else {
			fmt.Println("Acquire timeout")
		}
		g.Done()
	}()
	g.Wait()
	if n := s.DrainPermits(); n != 10 {
		t.Fail()
		s.ReleaseMany(n)
	} else {
		s.ReleaseMany(10)
	}
}

func aq(s *Semaphore, i int) {
	fmt.Println("Waiting to acquire", i, "permits,  avail:", s.AvailablePermits())
	s.AcquireMany(i)
	fmt.Println("Acquired", i, "permits, avail:", s.AvailablePermits())
	time.Sleep(time.Second * 3)
	s.ReleaseMany(i)
	fmt.Println("Done. Released", i, "permits, avail:", s.AvailablePermits())
	g.Done()
}
