package suture

// sum type pattern for type-safe message passing; see
// http://www.jerf.org/iri/post/2917

type supervisorMessage interface {
	isSupervisorMessage()
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
	s.sendControl(serviceEnded{id})
}

type serviceEnded struct {
	id serviceID
}

func (s serviceEnded) isSupervisorMessage() {}

// added by the Add() method
type addService struct {
	service  Service
	name     string
	response chan serviceID
}

func (as addService) isSupervisorMessage() {}

// Stop stops the Supervisor.
//
// This function will not return until either all Services have stopped, or
// they timeout after the timeout value given to the Supervisor at creation.
func (s *Supervisor) Stop() {
	done := make(chan struct{})
	if s.sendControl(stopSupervisor{done}) {
		<-done
	}
}

type stopSupervisor struct {
	done chan struct{}
}

func (ss stopSupervisor) isSupervisorMessage() {}

func (s *Supervisor) panic() {
	s.control <- panicSupervisor{}
}

type serviceTerminated struct {
	id serviceID
}

func (st serviceTerminated) isSupervisorMessage() {}

type panicSupervisor struct {
}

func (ps panicSupervisor) isSupervisorMessage() {}
