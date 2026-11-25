//go:build windows

package osutil

import (
	"context"

	"golang.org/x/sys/windows/svc"
)

func IsWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

type WindowsService struct {
	ctx    context.Context
	cancel context.CancelFunc

	mainFunc func(ctx context.Context)

	done chan struct{}
}

func NewWindowsService(mainFunc func(ctx context.Context)) *WindowsService {
	ctx, cancel := context.WithCancel(context.Background())
	return &WindowsService{
		ctx:      ctx,
		cancel:   cancel,
		mainFunc: mainFunc,
		done:     make(chan struct{}),
	}
}

// This function is automatically called by
// Windows Service Control Manager (SCM) on Start action.
// https://pkg.go.dev/golang.org/x/sys/windows/svc#Handler
func (s *WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const acceptedCommands = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	go func() {
		defer close(s.done)
		if s.mainFunc != nil {
			s.mainFunc(s.ctx)
		}
	}()

	status <- svc.Status{State: svc.Running, Accepts: acceptedCommands}

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			status <- req.CurrentStatus

		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}

			s.cancel()

			<-s.done

			status <- svc.Status{State: svc.Stopped}
			return false, 0

		default:
		}
	}

	return false, 0
}

func RunService(name string, mainFunc func(ctx context.Context)) error {
	service := NewWindowsService(mainFunc)
	return svc.Run(name, service)
}
