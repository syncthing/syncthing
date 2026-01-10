//go:build windows

package osutil

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func IsWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

func RunService(name string, handler svc.Handler) error {
	return svc.Run(name, handler)
}

func InstallService(name, displayName string, args ...string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	exepath, err := os.Executable()
	if err != nil {
		return err
	}

	s, err := m.CreateService(name, exepath, mgr.Config{DisplayName: displayName}, args...)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := eventlog.InstallAsEventCreate(name, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		s.Delete()
		return err
	}

	return nil
}

func RemoveService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer s.Close()

	if err = s.Delete(); err != nil {
		return err
	}
	_ = eventlog.Remove(name)
	return nil
}

type WindowsServiceHandler struct {
	MainFunc func(ctx context.Context)
}

func (m *WindowsServiceHandler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if m.MainFunc != nil {
			m.MainFunc(ctx)
		}
	}()

	s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	stopped := false
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			_ = exec.Command("taskkill", "/IM", "syncthing.exe", "/F").Run()
			cancel()
			stopped = true
			break
		default:
		}
		if stopped {
			break
		}
	}

	wg.Wait()

	s <- svc.Status{State: svc.StopPending}
	return false, 0
}
