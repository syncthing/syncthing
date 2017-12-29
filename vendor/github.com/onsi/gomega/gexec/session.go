/*
Package gexec provides support for testing external processes.
*/
package gexec

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

const INVALID_EXIT_CODE = 254

type Session struct {
	//The wrapped command
	Command *exec.Cmd

	//A *gbytes.Buffer connected to the command's stdout
	Out *gbytes.Buffer

	//A *gbytes.Buffer connected to the command's stderr
	Err *gbytes.Buffer

	//A channel that will close when the command exits
	Exited <-chan struct{}

	lock     *sync.Mutex
	exitCode int
}

/*
Start starts the passed-in *exec.Cmd command.  It wraps the command in a *gexec.Session.

The session pipes the command's stdout and stderr to two *gbytes.Buffers available as properties on the session: session.Out and session.Err.
These buffers can be used with the gbytes.Say matcher to match against unread output:

	Ω(session.Out).Should(gbytes.Say("foo-out"))
	Ω(session.Err).Should(gbytes.Say("foo-err"))

In addition, Session satisfies the gbytes.BufferProvider interface and provides the stdout *gbytes.Buffer.  This allows you to replace the first line, above, with:

	Ω(session).Should(gbytes.Say("foo-out"))

When outWriter and/or errWriter are non-nil, the session will pipe stdout and/or stderr output both into the session *gybtes.Buffers and to the passed-in outWriter/errWriter.
This is useful for capturing the process's output or logging it to screen.  In particular, when using Ginkgo it can be convenient to direct output to the GinkgoWriter:

	session, err := Start(command, GinkgoWriter, GinkgoWriter)

This will log output when running tests in verbose mode, but - otherwise - will only log output when a test fails.

The session wrapper is responsible for waiting on the *exec.Cmd command.  You *should not* call command.Wait() yourself.
Instead, to assert that the command has exited you can use the gexec.Exit matcher:

	Ω(session).Should(gexec.Exit())

When the session exits it closes the stdout and stderr gbytes buffers.  This will short circuit any
Eventuallys waiting for the buffers to Say something.
*/
func Start(command *exec.Cmd, outWriter io.Writer, errWriter io.Writer) (*Session, error) {
	exited := make(chan struct{})

	session := &Session{
		Command:  command,
		Out:      gbytes.NewBuffer(),
		Err:      gbytes.NewBuffer(),
		Exited:   exited,
		lock:     &sync.Mutex{},
		exitCode: -1,
	}

	var commandOut, commandErr io.Writer

	commandOut, commandErr = session.Out, session.Err

	if outWriter != nil {
		commandOut = io.MultiWriter(commandOut, outWriter)
	}

	if errWriter != nil {
		commandErr = io.MultiWriter(commandErr, errWriter)
	}

	command.Stdout = commandOut
	command.Stderr = commandErr

	err := command.Start()
	if err == nil {
		go session.monitorForExit(exited)
		trackedSessionsMutex.Lock()
		defer trackedSessionsMutex.Unlock()
		trackedSessions = append(trackedSessions, session)
	}

	return session, err
}

/*
Buffer implements the gbytes.BufferProvider interface and returns s.Out
This allows you to make gbytes.Say matcher assertions against stdout without having to reference .Out:

	Eventually(session).Should(gbytes.Say("foo"))
*/
func (s *Session) Buffer() *gbytes.Buffer {
	return s.Out
}

/*
ExitCode returns the wrapped command's exit code.  If the command hasn't exited yet, ExitCode returns -1.

To assert that the command has exited it is more convenient to use the Exit matcher:

	Eventually(s).Should(gexec.Exit())

When the process exits because it has received a particular signal, the exit code will be 128+signal-value
(See http://www.tldp.org/LDP/abs/html/exitcodes.html and http://man7.org/linux/man-pages/man7/signal.7.html)

*/
func (s *Session) ExitCode() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.exitCode
}

/*
Wait waits until the wrapped command exits.  It can be passed an optional timeout.
If the command does not exit within the timeout, Wait will trigger a test failure.

Wait returns the session, making it possible to chain:

	session.Wait().Out.Contents()

will wait for the command to exit then return the entirety of Out's contents.

Wait uses eventually under the hood and accepts the same timeout/polling intervals that eventually does.
*/
func (s *Session) Wait(timeout ...interface{}) *Session {
	EventuallyWithOffset(1, s, timeout...).Should(Exit())
	return s
}

/*
Kill sends the running command a SIGKILL signal.  It does not wait for the process to exit.

If the command has already exited, Kill returns silently.

The session is returned to enable chaining.
*/
func (s *Session) Kill() *Session {
	if s.ExitCode() != -1 {
		return s
	}
	s.Command.Process.Kill()
	return s
}

/*
Interrupt sends the running command a SIGINT signal.  It does not wait for the process to exit.

If the command has already exited, Interrupt returns silently.

The session is returned to enable chaining.
*/
func (s *Session) Interrupt() *Session {
	return s.Signal(syscall.SIGINT)
}

/*
Terminate sends the running command a SIGTERM signal.  It does not wait for the process to exit.

If the command has already exited, Terminate returns silently.

The session is returned to enable chaining.
*/
func (s *Session) Terminate() *Session {
	return s.Signal(syscall.SIGTERM)
}

/*
Signal sends the running command the passed in signal.  It does not wait for the process to exit.

If the command has already exited, Signal returns silently.

The session is returned to enable chaining.
*/
func (s *Session) Signal(signal os.Signal) *Session {
	if s.ExitCode() != -1 {
		return s
	}
	s.Command.Process.Signal(signal)
	return s
}

func (s *Session) monitorForExit(exited chan<- struct{}) {
	err := s.Command.Wait()
	s.lock.Lock()
	s.Out.Close()
	s.Err.Close()
	status := s.Command.ProcessState.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		s.exitCode = 128 + int(status.Signal())
	} else {
		exitStatus := status.ExitStatus()
		if exitStatus == -1 && err != nil {
			s.exitCode = INVALID_EXIT_CODE
		}
		s.exitCode = exitStatus
	}
	s.lock.Unlock()

	close(exited)
}

var trackedSessions = []*Session{}
var trackedSessionsMutex = &sync.Mutex{}

/*
Kill sends a SIGKILL signal to all the processes started by Run, and waits for them to exit.
The timeout specified is applied to each process killed.

If any of the processes already exited, KillAndWait returns silently.
*/
func KillAndWait(timeout ...interface{}) {
	trackedSessionsMutex.Lock()
	defer trackedSessionsMutex.Unlock()
	for _, session := range trackedSessions {
		session.Kill().Wait(timeout...)
	}
	trackedSessions = []*Session{}
}

/*
Kill sends a SIGTERM signal to all the processes started by Run, and waits for them to exit.
The timeout specified is applied to each process killed.

If any of the processes already exited, TerminateAndWait returns silently.
*/
func TerminateAndWait(timeout ...interface{}) {
	trackedSessionsMutex.Lock()
	defer trackedSessionsMutex.Unlock()
	for _, session := range trackedSessions {
		session.Terminate().Wait(timeout...)
	}
}

/*
Kill sends a SIGKILL signal to all the processes started by Run.
It does not wait for the processes to exit.

If any of the processes already exited, Kill returns silently.
*/
func Kill() {
	trackedSessionsMutex.Lock()
	defer trackedSessionsMutex.Unlock()
	for _, session := range trackedSessions {
		session.Kill()
	}
}

/*
Terminate sends a SIGTERM signal to all the processes started by Run.
It does not wait for the processes to exit.

If any of the processes already exited, Terminate returns silently.
*/
func Terminate() {
	trackedSessionsMutex.Lock()
	defer trackedSessionsMutex.Unlock()
	for _, session := range trackedSessions {
		session.Terminate()
	}
}

/*
Signal sends the passed in signal to all the processes started by Run.
It does not wait for the processes to exit.

If any of the processes already exited, Signal returns silently.
*/
func Signal(signal os.Signal) {
	trackedSessionsMutex.Lock()
	defer trackedSessionsMutex.Unlock()
	for _, session := range trackedSessions {
		session.Signal(signal)
	}
}

/*
Interrupt sends the SIGINT signal to all the processes started by Run.
It does not wait for the processes to exit.

If any of the processes already exited, Interrupt returns silently.
*/
func Interrupt() {
	trackedSessionsMutex.Lock()
	defer trackedSessionsMutex.Unlock()
	for _, session := range trackedSessions {
		session.Interrupt()
	}
}
