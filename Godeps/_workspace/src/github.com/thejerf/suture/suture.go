/*

Package suture provides Erlang-like supervisor trees.

This implements Erlang-esque supervisor trees, as adapted for Go. This is
intended to be an industrial-strength implementation, but it has not yet
been deployed in a hostile environment. (It's headed there, though.)

Supervisor Tree -> SuTree -> suture -> holds your code together when it's
trying to fall apart.

Why use Suture?

 * You want to write bullet-resistant services that will remain available
   despite unforeseen failure.
 * You need the code to be smart enough not to consume 100% of the CPU
   restarting things.
 * You want to easily compose multiple such services in one program.
 * You want the Erlang programmers to stop lording their supervision
   trees over you.

Suture has 100% test coverage, and is golint clean. This doesn't prove it
free of bugs, but it shows I care.

A blog post describing the design decisions is available at
http://www.jerf.org/iri/post/2930 .

Using Suture

To idiomatically use Suture, create a Supervisor which is your top level
"application" supervisor. This will often occur in your program's "main"
function.

Create "Service"s, which implement the Service interface. .Add() them
to your Supervisor. Supervisors are also services, so you can create a
tree structure here, depending on the exact combination of restarts
you want to create.

As a special case, when adding Supervisors to Supervisors, the "sub"
supervisor will have the "super" supervisor's Log function copied.
This allows you to set one log function on the "top" supervisor, and
have it propagate down to all the sub-supervisors. This also allows
libraries or modules to provide Supervisors without having to commit
their users to a particular logging method.

Finally, as what is probably the last line of your main() function, call
.Serve() on your top level supervisor. This will start all the services
you've defined.

See the Example for an example, using a simple service that serves out
incrementing integers.

*/
package suture

import (
	"errors"
	"fmt"
	"log"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	notRunning = iota
	normal
	paused
)

type supervisorID uint32
type serviceID uint32

var currentSupervisorID uint32

// ErrWrongSupervisor is returned by the (*Supervisor).Remove method
// if you pass a ServiceToken from the wrong Supervisor.
var ErrWrongSupervisor = errors.New("wrong supervisor for this service token, no service removed")

// ServiceToken is an opaque identifier that can be used to terminate a service that
// has been Add()ed to a Supervisor.
type ServiceToken struct {
	id uint64
}

/*
Supervisor is the core type of the module that represents a Supervisor.

Supervisors should be constructed either by New or NewSimple.

Once constructed, a Supervisor should be started in one of three ways:

 1. Calling .Serve().
 2. Calling .ServeBackground().
 3. Adding it to an existing Supervisor.

Calling Serve will cause the supervisor to run until it is shut down by
an external user calling Stop() on it. If that never happens, it simply
runs forever. I suggest creating your services in Supervisors, then making
a Serve() call on your top-level Supervisor be the last line of your main
func.

Calling ServeBackground will CORRECTLY start the supervisor running in a
new goroutine. You do not want to just:

  go supervisor.Serve()

because that will briefly create a race condition as it starts up, if you
try to .Add() services immediately afterward.

*/
type Supervisor struct {
	Name string
	id   supervisorID

	failureDecay     float64
	failureThreshold float64
	failureBackoff   time.Duration
	timeout          time.Duration
	log              func(string)
	services         map[serviceID]Service
	lastFail         time.Time
	failures         float64
	restartQueue     []serviceID
	serviceCounter   serviceID
	control          chan supervisorMessage
	resumeTimer      <-chan time.Time

	// The testing uses the ability to grab these individual logging functions
	// and get inside of suture's handling at a deep level.
	// If you ever come up with some need to get into these, submit a pull
	// request to make them public and some smidge of justification, and
	// I'll happily do it.
	// But since I've now changed the signature on these once, I'm glad I
	// didn't start with them public... :)
	logBadStop func(*Supervisor, Service)
	logFailure func(supervisor *Supervisor, service Service, currentFailures float64, failureThreshold float64, restarting bool, error interface{}, stacktrace []byte)
	logBackoff func(*Supervisor, bool)

	// avoid a dependency on github.com/thejerf/abtime by just implementing
	// a minimal chunk.
	getNow    func() time.Time
	getResume func(time.Duration) <-chan time.Time

	sync.Mutex
	state uint8
}

// Spec is used to pass arguments to the New function to create a
// supervisor. See the New function for full documentation.
type Spec struct {
	Log              func(string)
	FailureDecay     float64
	FailureThreshold float64
	FailureBackoff   time.Duration
	Timeout          time.Duration
}

/*

New is the full constructor function for a supervisor.

The name is a friendly human name for the supervisor, used in logging. Suture
does not care if this is unique, but it is good for your sanity if it is.

If not set, the following values are used:

 * Log:               A function is created that uses log.Print.
 * FailureDecay:      30 seconds
 * FailureThreshold:  5 failures
 * FailureBackoff:    15 seconds
 * Timeout:           10 seconds

The Log function will be called when errors occur. Suture will log the
following:

 * When a service has failed, with a descriptive message about the
   current backoff status, and whether it was immediately restarted
 * When the supervisor has gone into its backoff mode, and when it
   exits it
 * When a service fails to stop

The failureRate, failureThreshold, and failureBackoff controls how failures
are handled, in order to avoid the supervisor failure case where the
program does nothing but restarting failed services. If you do not
care how failures behave, the default values should be fine for the
vast majority of services, but if you want the details:

The supervisor tracks the number of failures that have occurred, with an
exponential decay on the count. Every FailureDecay seconds, the number of
failures that have occurred is cut in half. (This is done smoothly with an
exponential function.) When a failure occurs, the number of failures
is incremented by one. When the number of failures passes the
FailureThreshold, the entire service waits for FailureBackoff seconds
before attempting any further restarts, at which point it resets its
failure count to zero.

Timeout is how long Suture will wait for a service to properly terminate.

*/
func New(name string, spec Spec) (s *Supervisor) {
	s = new(Supervisor)

	s.Name = name
	s.id = supervisorID(atomic.AddUint32(&currentSupervisorID, 1))

	if spec.Log == nil {
		s.log = func(msg string) {
			log.Print(fmt.Sprintf("Supervisor %s: %s", s.Name, msg))
		}
	} else {
		s.log = spec.Log
	}

	if spec.FailureDecay == 0 {
		s.failureDecay = 30
	} else {
		s.failureDecay = spec.FailureDecay
	}
	if spec.FailureThreshold == 0 {
		s.failureThreshold = 5
	} else {
		s.failureThreshold = spec.FailureThreshold
	}
	if spec.FailureBackoff == 0 {
		s.failureBackoff = time.Second * 15
	} else {
		s.failureBackoff = spec.FailureBackoff
	}
	if spec.Timeout == 0 {
		s.timeout = time.Second * 10
	} else {
		s.timeout = spec.Timeout
	}

	// overriding these allows for testing the threshold behavior
	s.getNow = time.Now
	s.getResume = time.After

	s.control = make(chan supervisorMessage)
	s.services = make(map[serviceID]Service)
	s.restartQueue = make([]serviceID, 0, 1)
	s.resumeTimer = make(chan time.Time)

	// set up the default logging handlers
	s.logBadStop = func(supervisor *Supervisor, service Service) {
		s.log(fmt.Sprintf("%s: Service %s failed to terminate in a timely manner", serviceName(supervisor), serviceName(service)))
	}
	s.logFailure = func(supervisor *Supervisor, service Service, failures float64, threshold float64, restarting bool, err interface{}, st []byte) {
		var errString string

		e, canError := err.(error)
		if canError {
			errString = e.Error()
		} else {
			errString = fmt.Sprintf("%#v", err)
		}

		s.log(fmt.Sprintf("%s: Failed service '%s' (%f failures of %f), restarting: %#v, error: %s, stacktrace: %s", serviceName(supervisor), serviceName(service), failures, threshold, restarting, errString, string(st)))
	}
	s.logBackoff = func(s *Supervisor, entering bool) {
		if entering {
			s.log("Entering the backoff state.")
		} else {
			s.log("Exiting backoff state.")
		}
	}

	return
}

func serviceName(service Service) (serviceName string) {
	stringer, canStringer := service.(fmt.Stringer)
	if canStringer {
		serviceName = stringer.String()
	} else {
		serviceName = fmt.Sprintf("%#v", service)
	}
	return
}

// NewSimple is a convenience function to create a service with just a name
// and the sensible defaults.
func NewSimple(name string) *Supervisor {
	return New(name, Spec{})
}

/*
Service is the interface that describes a service to a Supervisor.

Serve Method

The Serve method is called by a Supervisor to start the service.
The service should execute within the goroutine that this is
called in. If this function either returns or panics, the Supervisor
will call it again.

A Serve method SHOULD do as much cleanup of the state as possible,
to prevent any corruption in the previous state from crashing the
service again.

Stop Method

This method is used by the supervisor to stop the service. Calling this
directly on a Service given to a Supervisor will simply result in the
Service being restarted; use the Supervisor's .Remove(ServiceToken) method
to stop a service. A supervisor will call .Stop() only once. Thus, it may
be as destructive as it likes to get the service to stop.

Once Stop has been called on a Service, the Service SHOULD NOT be
reused in any other supervisor! Because of the impossibility of
guaranteeing that the service has actually stopped in Go, you can't
prove that you won't be starting two goroutines using the exact
same memory to store state, causing completely unpredictable behavior.

Stop should not return until the service has actually stopped.
"Stopped" here is defined as "the service will stop servicing any
further requests in the future". For instance, a common implementation
is to receive a message on a dedicated "stop" channel and immediately
returning. Once the stop command has been processed, the service is
stopped.

Another common Stop implementation is to forcibly close an open socket
or other resource, which will cause detectable errors to manifest in the
service code. Bear in mind that to perfectly correctly use this
approach requires a bit more work to handle the chance of a Stop
command coming in before the resource has been created.

If a service does not Stop within the supervisor's timeout duration, a log
entry will be made with a descriptive string to that effect. This does
not guarantee that the service is hung; it may still get around to being
properly stopped in the future. Until the service is fully stopped,
both the service and the spawned goroutine trying to stop it will be
"leaked".

Stringer Interface

It is not mandatory to implement the fmt.Stringer interface on your
service, but if your Service does happen to implement that, the log
messages that describe your service will use that when naming the
service. Otherwise, you'll see the GoString of your service object,
obtained via fmt.Sprintf("%#v", service).

*/
type Service interface {
	Serve()
	Stop()
}

/*
Add adds a service to this supervisor.

If the supervisor is currently running, the service will be started
immediately. If the supervisor is not currently running, the service
will be started when the supervisor is.

The returned ServiceID may be passed to the Remove method of the Supervisor
to terminate the service.

As a special behavior, if the service added is itself a supervisor, the
supervisor being added will copy the Log function from the Supervisor it
is being added to. This allows factoring out providing a Supervisor
from its logging.

*/
func (s *Supervisor) Add(service Service) ServiceToken {
	if s == nil {
		panic("can't add service to nil *suture.Supervisor")
	}

	if supervisor, isSupervisor := service.(*Supervisor); isSupervisor {
		supervisor.logBadStop = s.logBadStop
		supervisor.logFailure = s.logFailure
		supervisor.logBackoff = s.logBackoff
	}

	s.Lock()
	if s.state == notRunning {
		id := s.serviceCounter
		s.serviceCounter++

		s.services[id] = service
		s.restartQueue = append(s.restartQueue, id)

		s.Unlock()
		return ServiceToken{uint64(s.id)<<32 | uint64(id)}
	}
	s.Unlock()

	response := make(chan serviceID)
	s.control <- addService{service, response}
	return ServiceToken{uint64(s.id)<<32 | uint64(<-response)}
}

// ServeBackground starts running a supervisor in its own goroutine. This
// method does not return until it is safe to use .Add() on the Supervisor.
func (s *Supervisor) ServeBackground() {
	go s.Serve()
	s.sync()
}

/*
Serve starts the supervisor. You should call this on the top-level supervisor,
but nothing else.
*/
func (s *Supervisor) Serve() {
	if s == nil {
		panic("Can't serve with a nil *suture.Supervisor")
	}
	if s.id == 0 {
		panic("Can't call Serve on an incorrectly-constructed *suture.Supervisor")
	}

	defer func() {
		s.Lock()
		s.state = notRunning
		s.Unlock()
	}()

	s.Lock()
	if s.state != notRunning {
		s.Unlock()
		panic("Running a supervisor while it is already running?")
	}

	s.state = normal
	s.Unlock()

	// for all the services I currently know about, start them
	for _, id := range s.restartQueue {
		service, present := s.services[id]
		if present {
			s.runService(service, id)
		}
	}
	s.restartQueue = make([]serviceID, 0, 1)

	for {
		select {
		case m := <-s.control:
			switch msg := m.(type) {
			case serviceFailed:
				s.handleFailedService(msg.id, msg.err, msg.stacktrace)
			case serviceEnded:
				service, monitored := s.services[msg.id]
				if monitored {
					s.handleFailedService(msg.id, fmt.Sprintf("%s returned unexpectedly", service), []byte("[unknown stack trace]"))
				}
			case addService:
				id := s.serviceCounter
				s.serviceCounter++

				s.services[id] = msg.service
				s.runService(msg.service, id)

				msg.response <- id
			case removeService:
				s.removeService(msg.id)
			case stopSupervisor:
				for id := range s.services {
					s.removeService(id)
				}
				return
			case listServices:
				services := []Service{}
				for _, service := range s.services {
					services = append(services, service)
				}
				msg.c <- services
			case syncSupervisor:
				// this does nothing on purpose; its sole purpose is to
				// introduce a sync point via the channel receive
			case panicSupervisor:
				// used only by tests
				panic("Panicking as requested!")
			}
		case _ = <-s.resumeTimer:
			// We're resuming normal operation after a pause due to
			// excessive thrashing
			// FIXME: Ought to permit some spacing of these functions, rather
			// than simply hammering through them
			s.Lock()
			s.state = normal
			s.Unlock()
			s.failures = 0
			s.logBackoff(s, false)
			for _, id := range s.restartQueue {
				service, present := s.services[id]
				if present {
					s.runService(service, id)
				}
			}
			s.restartQueue = make([]serviceID, 0, 1)
		}
	}
}

func (s *Supervisor) handleFailedService(id serviceID, err interface{}, stacktrace []byte) {
	now := s.getNow()

	if s.lastFail.IsZero() {
		s.lastFail = now
		s.failures = 1.0
	} else {
		sinceLastFail := now.Sub(s.lastFail).Seconds()
		intervals := sinceLastFail / s.failureDecay
		s.failures = s.failures*math.Pow(.5, intervals) + 1
	}

	if s.failures > s.failureThreshold {
		s.Lock()
		s.state = paused
		s.Unlock()
		s.logBackoff(s, true)
		s.resumeTimer = s.getResume(s.failureBackoff)
	}

	s.lastFail = now

	failedService, monitored := s.services[id]

	// It is possible for a service to be no longer monitored
	// by the time we get here. In that case, just ignore it.
	if monitored {
		// this may look dangerous because the state could change, but this
		// code is only ever run in the one goroutine that is permitted to
		// change the state, so nothing else will.
		s.Lock()
		curState := s.state
		s.Unlock()
		if curState == normal {
			s.runService(failedService, id)
			s.logFailure(s, failedService, s.failures, s.failureThreshold, true, err, stacktrace)
		} else {
			// FIXME: When restarting, check that the service still
			// exists (it may have been stopped in the meantime)
			s.restartQueue = append(s.restartQueue, id)
			s.logFailure(s, failedService, s.failures, s.failureThreshold, false, err, stacktrace)
		}
	}
}

func (s *Supervisor) runService(service Service, id serviceID) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 65535, 65535)
				written := runtime.Stack(buf, false)
				buf = buf[:written]
				s.fail(id, r, buf)
			}
		}()

		service.Serve()

		s.serviceEnded(id)
	}()
}

func (s *Supervisor) removeService(id serviceID) {
	service, present := s.services[id]
	if present {
		delete(s.services, id)
		go func() {
			successChan := make(chan bool)
			go func() {
				service.Stop()
				successChan <- true
			}()

			failChan := s.getResume(s.timeout)

			select {
			case <-successChan:
				// Life is good!
			case <-failChan:
				s.logBadStop(s, service)
			}
		}()
	}
}

// String implements the fmt.Stringer interface.
func (s *Supervisor) String() string {
	return s.Name
}

// sum type pattern for type-safe message passing; see
// http://www.jerf.org/iri/post/2917

type supervisorMessage interface {
	isSupervisorMessage()
}

/*
Remove will remove the given service from the Supervisor, and attempt to Stop() it.
The ServiceID token comes from the Add() call.
*/
func (s *Supervisor) Remove(id ServiceToken) error {
	sID := supervisorID(id.id >> 32)
	if sID != s.id {
		return ErrWrongSupervisor
	}
	s.control <- removeService{serviceID(id.id & 0xffffffff)}
	return nil
}

/*

Services returns a []Service containing a snapshot of the services this
Supervisor is managing.

*/
func (s *Supervisor) Services() []Service {
	ls := listServices{make(chan []Service)}
	s.control <- ls
	return <-ls.c
}

type listServices struct {
	c chan []Service
}

func (ls listServices) isSupervisorMessage() {}

type removeService struct {
	id serviceID
}

func (rs removeService) isSupervisorMessage() {}

func (s *Supervisor) sync() {
	s.control <- syncSupervisor{}
}

type syncSupervisor struct {
}

func (ss syncSupervisor) isSupervisorMessage() {}

func (s *Supervisor) fail(id serviceID, err interface{}, stacktrace []byte) {
	s.control <- serviceFailed{id, err, stacktrace}
}

type serviceFailed struct {
	id         serviceID
	err        interface{}
	stacktrace []byte
}

func (sf serviceFailed) isSupervisorMessage() {}

func (s *Supervisor) serviceEnded(id serviceID) {
	s.control <- serviceEnded{id}
}

type serviceEnded struct {
	id serviceID
}

func (s serviceEnded) isSupervisorMessage() {}

// added by the Add() method
type addService struct {
	service  Service
	response chan serviceID
}

func (as addService) isSupervisorMessage() {}

// Stop stops the Supervisor.
func (s *Supervisor) Stop() {
	s.control <- stopSupervisor{}
}

type stopSupervisor struct {
}

func (ss stopSupervisor) isSupervisorMessage() {}

func (s *Supervisor) panic() {
	s.control <- panicSupervisor{}
}

type panicSupervisor struct {
}

func (ps panicSupervisor) isSupervisorMessage() {}
