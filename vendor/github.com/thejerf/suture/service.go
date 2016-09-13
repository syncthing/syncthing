package suture

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

When a Service is added to a Supervisor, the Supervisor will create a
string representation of that service used for logging.

If you implement the fmt.Stringer interface, that will be used.

If you do not implement the fmt.Stringer interface, a default
fmt.Sprintf("%#v") will be used.

*/
type Service interface {
	Serve()
	Stop()
}
