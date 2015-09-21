package suture

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

const (
	Happy = iota
	Fail
	Panic
	Hang
	UseStopChan
)

var everMultistarted = false

// Test that supervisors work perfectly when everything is hunky dory.
func TestTheHappyCase(t *testing.T) {
	t.Parallel()

	s := NewSimple("A")
	if s.String() != "A" {
		t.Fatal("Can't get name from a supervisor")
	}
	service := NewService("B")

	s.Add(service)

	go s.Serve()

	<-service.started

	// If we stop the service, it just gets restarted
	service.Stop()
	<-service.started

	// And it is shut down when we stop the supervisor
	service.take <- UseStopChan
	s.Stop()
	<-service.stop
}

// Test that adding to a running supervisor does indeed start the service.
func TestAddingToRunningSupervisor(t *testing.T) {
	t.Parallel()

	s := NewSimple("A1")

	s.ServeBackground()
	defer s.Stop()

	service := NewService("B1")
	s.Add(service)

	<-service.started

	services := s.Services()
	if !reflect.DeepEqual([]Service{service}, services) {
		t.Fatal("Can't get list of services as expected.")
	}
}

// Test what happens when services fail.
func TestFailures(t *testing.T) {
	t.Parallel()

	s := NewSimple("A2")
	s.failureThreshold = 3.5

	go s.Serve()
	defer func() {
		// to avoid deadlocks during shutdown, we have to not try to send
		// things out on channels while we're shutting down (this undoes the
		// logFailure overide about 25 lines down)
		s.logFailure = func(*Supervisor, Service, float64, float64, bool, interface{}, []byte) {}
		s.Stop()
	}()
	s.sync()

	service1 := NewService("B2")
	service2 := NewService("C2")

	s.Add(service1)
	<-service1.started
	s.Add(service2)
	<-service2.started

	nowFeeder := NewNowFeeder()
	pastVal := time.Unix(1000000, 0)
	nowFeeder.appendTimes(pastVal)
	s.getNow = nowFeeder.getter

	resumeChan := make(chan time.Time)
	s.getResume = func(d time.Duration) <-chan time.Time {
		return resumeChan
	}

	failNotify := make(chan bool)
	// use this to synchronize on here
	s.logFailure = func(supervisor *Supervisor, s Service, cf float64, ft float64, r bool, error interface{}, stacktrace []byte) {
		failNotify <- r
	}

	// All that setup was for this: Service1, please return now.
	service1.take <- Fail
	restarted := <-failNotify
	<-service1.started

	if !restarted || s.failures != 1 || s.lastFail != pastVal {
		t.Fatal("Did not fail in the expected manner")
	}
	// Getting past this means the service was restarted.
	service1.take <- Happy

	// Service2, your turn.
	service2.take <- Fail
	nowFeeder.appendTimes(pastVal)
	restarted = <-failNotify
	<-service2.started
	if !restarted || s.failures != 2 || s.lastFail != pastVal {
		t.Fatal("Did not fail in the expected manner")
	}
	// And you're back. (That is, the correct service was restarted.)
	service2.take <- Happy

	// Now, one failureDecay later, is everything working correctly?
	oneDecayLater := time.Unix(1000030, 0)
	nowFeeder.appendTimes(oneDecayLater)
	service2.take <- Fail
	restarted = <-failNotify
	<-service2.started
	// playing a bit fast and loose here with floating point, but...
	// we get 2 by taking the current failure value of 2, decaying it
	// by one interval, which cuts it in half to 1, then adding 1 again,
	// all of which "should" be precise
	if !restarted || s.failures != 2 || s.lastFail != oneDecayLater {
		t.Fatal("Did not decay properly", s.lastFail, oneDecayLater)
	}

	// For a change of pace, service1 would you be so kind as to panic?
	nowFeeder.appendTimes(oneDecayLater)
	service1.take <- Panic
	restarted = <-failNotify
	<-service1.started
	if !restarted || s.failures != 3 || s.lastFail != oneDecayLater {
		t.Fatal("Did not correctly recover from a panic")
	}

	nowFeeder.appendTimes(oneDecayLater)
	backingoff := make(chan bool)
	s.logBackoff = func(s *Supervisor, backingOff bool) {
		backingoff <- backingOff
	}

	// And with this failure, we trigger the backoff code.
	service1.take <- Fail
	backoff := <-backingoff
	restarted = <-failNotify

	if !backoff || restarted || s.failures != 4 {
		t.Fatal("Broke past the threshold but did not log correctly", s.failures)
	}
	if service1.existing != 0 {
		t.Fatal("service1 still exists according to itself?")
	}

	// service2 is still running, because we don't shut anything down in a
	// backoff, we just stop restarting.
	service2.take <- Happy

	var correct bool
	timer := time.NewTimer(time.Millisecond * 10)
	// verify the service has not been restarted
	// hard to get around race conditions here without simply using a timer...
	select {
	case service1.take <- Happy:
		correct = false
	case <-timer.C:
		correct = true
	}
	if !correct {
		t.Fatal("Restarted the service during the backoff interval")
	}

	// tell the supervisor the restart interval has passed
	resumeChan <- time.Time{}
	backoff = <-backingoff
	<-service1.started
	s.sync()
	if s.failures != 0 {
		t.Fatal("Did not reset failure count after coming back from timeout.")
	}

	nowFeeder.appendTimes(oneDecayLater)
	service1.take <- Fail
	restarted = <-failNotify
	<-service1.started
	if !restarted || backoff {
		t.Fatal("For some reason, got that we were backing off again.", restarted, backoff)
	}
}

func TestRunningAlreadyRunning(t *testing.T) {
	t.Parallel()

	s := NewSimple("A3")
	go s.Serve()
	defer s.Stop()

	// ensure the supervisor has made it to its main loop
	s.sync()
	var errored bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				errored = true
			}
		}()

		s.Serve()
	}()
	if !errored {
		t.Fatal("Supervisor failed to prevent itself from double-running.")
	}
}

func TestFullConstruction(t *testing.T) {
	t.Parallel()

	s := New("Moo", Spec{
		Log:              func(string) {},
		FailureDecay:     1,
		FailureThreshold: 2,
		FailureBackoff:   3,
		Timeout:          time.Second * 29,
	})
	if s.String() != "Moo" || s.failureDecay != 1 || s.failureThreshold != 2 || s.failureBackoff != 3 || s.timeout != time.Second*29 {
		t.Fatal("Full construction failed somehow")
	}
}

// This is mostly for coverage testing.
func TestDefaultLogging(t *testing.T) {
	t.Parallel()

	s := NewSimple("A4")

	service := NewService("B4")
	s.Add(service)

	s.failureThreshold = .5
	s.failureBackoff = time.Millisecond * 25
	go s.Serve()
	s.sync()

	<-service.started

	resumeChan := make(chan time.Time)
	s.getResume = func(d time.Duration) <-chan time.Time {
		return resumeChan
	}

	service.take <- UseStopChan
	service.take <- Fail
	<-service.stop
	resumeChan <- time.Time{}

	<-service.started

	service.take <- Happy

	serviceName(&BarelyService{})

	s.logBadStop(s, service)
	s.logFailure(s, service, 1, 1, true, errors.New("test error"), []byte{})

	s.Stop()
}

func TestNestedSupervisors(t *testing.T) {
	t.Parallel()

	super1 := NewSimple("Top5")
	super2 := NewSimple("Nested5")
	service := NewService("Service5")

	super2.logBadStop = func(*Supervisor, Service) {
		panic("Failed to copy logBadStop")
	}

	super1.Add(super2)
	super2.Add(service)

	// test the functions got copied from super1; if this panics, it didn't
	// get copied
	super2.logBadStop(super2, service)

	go super1.Serve()
	super1.sync()

	<-service.started
	service.take <- Happy

	super1.Stop()
}

func TestStoppingSupervisorStopsServices(t *testing.T) {
	t.Parallel()

	s := NewSimple("Top6")
	service := NewService("Service 6")

	s.Add(service)

	go s.Serve()
	s.sync()

	<-service.started

	service.take <- UseStopChan

	s.Stop()
	<-service.stop
}

func TestStoppingStillWorksWithHungServices(t *testing.T) {
	t.Parallel()

	s := NewSimple("Top7")
	service := NewService("Service WillHang7")

	s.Add(service)

	go s.Serve()

	<-service.started

	service.take <- UseStopChan
	service.take <- Hang

	resumeChan := make(chan time.Time)
	s.getResume = func(d time.Duration) <-chan time.Time {
		return resumeChan
	}
	failNotify := make(chan struct{})
	s.logBadStop = func(supervisor *Supervisor, s Service) {
		failNotify <- struct{}{}
	}

	s.Stop()

	resumeChan <- time.Time{}
	<-failNotify
	service.release <- true
	<-service.stop
}

func TestRemoveService(t *testing.T) {
	t.Parallel()

	s := NewSimple("Top")
	service := NewService("ServiceToRemove8")

	id := s.Add(service)

	go s.Serve()

	<-service.started
	service.take <- UseStopChan

	err := s.Remove(id)
	if err != nil {
		t.Fatal("Removing service somehow failed")
	}
	<-service.stop

	err = s.Remove(ServiceToken{1<<36 + 1})
	if err != ErrWrongSupervisor {
		t.Fatal("Did not detect that the ServiceToken was wrong")
	}
}

func TestFailureToConstruct(t *testing.T) {
	t.Parallel()

	var s *Supervisor

	panics(func() {
		s.Serve()
	})

	s = new(Supervisor)
	panics(func() {
		s.Serve()
	})
}

func TestFailingSupervisors(t *testing.T) {
	t.Parallel()

	// This is a bit of a complicated test, so let me explain what
	// all this is doing:
	// 1. Set up a top-level supervisor with a hair-trigger backoff.
	// 2. Add a supervisor to that.
	// 3. To that supervisor, add a service.
	// 4. Panic the supervisor in the middle, sending the top-level into
	//    backoff.
	// 5. Kill the lower level service too.
	// 6. Verify that when the top-level service comes out of backoff,
	//    the service ends up restarted as expected.

	// Ultimately, we can't have more than a best-effort recovery here.
	// A panic'ed supervisor can't really be trusted to have consistent state,
	// and without *that*, we can't trust it to do anything sensible with
	// the children it may have been running. So unlike Erlang, we can't
	// can't really expect to be able to safely restart them or anything.
	// Really, the "correct" answer is that the Supervisor must never panic,
	// but in the event that it does, this verifies that it at least tries
	// to get on with life.

	// This also tests that if a Supervisor itself panics, and one of its
	// monitored services goes down in the meantime, that the monitored
	// service also gets correctly restarted when the supervisor does.

	s1 := NewSimple("Top9")
	s2 := NewSimple("Nested9")
	service := NewService("Service9")

	s1.Add(s2)
	s2.Add(service)

	go s1.Serve()
	<-service.started

	s1.failureThreshold = .5

	// let us control precisely when s1 comes back
	resumeChan := make(chan time.Time)
	s1.getResume = func(d time.Duration) <-chan time.Time {
		return resumeChan
	}
	failNotify := make(chan string)
	// use this to synchronize on here
	s1.logFailure = func(supervisor *Supervisor, s Service, cf float64, ft float64, r bool, error interface{}, stacktrace []byte) {
		failNotify <- fmt.Sprintf("%s", s)
	}

	s2.panic()

	failing := <-failNotify
	// that's enough sync to guarantee this:
	if failing != "Nested9" || s1.state != paused {
		t.Fatal("Top-level supervisor did not go into backoff as expected")
	}

	service.take <- Fail

	resumeChan <- time.Time{}
	<-service.started
}

func TestNilSupervisorAdd(t *testing.T) {
	t.Parallel()

	var s *Supervisor

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("did not panic as expected on nil add")
		}
	}()

	s.Add(s)
}

// https://github.com/thejerf/suture/issues/11
//
// The purpose of this test is to verify that it does not cause data races,
// so there are no obvious assertions.
func TestIssue11(t *testing.T) {
	t.Parallel()

	s := NewSimple("main")
	s.ServeBackground()

	subsuper := NewSimple("sub")
	s.Add(subsuper)

	subsuper.Add(NewService("may cause data race"))
}

// http://golangtutorials.blogspot.com/2011/10/gotest-unit-testing-and-benchmarking-go.html
// claims test function are run in the same order as the source file...
// I'm not sure if this is part of the contract, though. Especially in the
// face of "t.Parallel()"...
func TestEverMultistarted(t *testing.T) {
	if everMultistarted {
		t.Fatal("Seem to have multistarted a service at some point, bummer.")
	}
}

// A test service that can be induced to fail, panic, or hang on demand.
func NewService(name string) *FailableService {
	return &FailableService{name, make(chan bool), make(chan int),
		make(chan bool, 1), make(chan bool), make(chan bool), 0}
}

type FailableService struct {
	name     string
	started  chan bool
	take     chan int
	shutdown chan bool
	release  chan bool
	stop     chan bool
	existing int
}

func (s *FailableService) Serve() {
	if s.existing != 0 {
		everMultistarted = true
		panic("Multi-started the same service! " + s.name)
	}
	s.existing++

	s.started <- true

	useStopChan := false

	for {
		select {
		case val := <-s.take:
			switch val {
			case Happy:
				// Do nothing on purpose. Life is good!
			case Fail:
				s.existing--
				if useStopChan {
					s.stop <- true
				}
				return
			case Panic:
				s.existing--
				panic("Panic!")
			case Hang:
				// or more specifically, "hang until I release you"
				<-s.release
			case UseStopChan:
				useStopChan = true
			}
		case <-s.shutdown:
			s.existing--
			if useStopChan {
				s.stop <- true
			}
			return
		}
	}
}

func (s *FailableService) String() string {
	return s.name
}

func (s *FailableService) Stop() {
	s.shutdown <- true
}

type NowFeeder struct {
	values []time.Time
	getter func() time.Time
	m      sync.Mutex
}

// This is used to test serviceName; it's a service without a Stringer.
type BarelyService struct{}

func (bs *BarelyService) Serve() {}
func (bs *BarelyService) Stop()  {}

func NewNowFeeder() (nf *NowFeeder) {
	nf = new(NowFeeder)
	nf.getter = func() time.Time {
		nf.m.Lock()
		defer nf.m.Unlock()
		if len(nf.values) > 0 {
			ret := nf.values[0]
			nf.values = nf.values[1:]
			return ret
		}
		panic("Ran out of values for NowFeeder")
	}
	return
}

func (nf *NowFeeder) appendTimes(t ...time.Time) {
	nf.m.Lock()
	defer nf.m.Unlock()
	nf.values = append(nf.values, t...)
}

func panics(doesItPanic func()) (panics bool) {
	defer func() {
		if r := recover(); r != nil {
			panics = true
		}
	}()

	doesItPanic()

	return
}
