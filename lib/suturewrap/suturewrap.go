// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package suturewrap

import (
	"context"
	"errors"
	"fmt"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture"
)

type FatalErr struct {
	Err    error
	Status ExitStatus
}

func (e *FatalErr) Error() string {
	return e.Err.Error()
}

func (e *FatalErr) Unwrap() error {
	return e.Err
}

type ExitStatus int

const (
	ExitSuccess            ExitStatus = 0
	ExitError              ExitStatus = 1
	ExitNoUpgradeAvailable ExitStatus = 2
	ExitRestart            ExitStatus = 3
	ExitUpgrade            ExitStatus = 4
)

func (s ExitStatus) AsInt() int {
	return int(s)
}

type ServiceToken suture.ServiceToken

type Service interface {
	Serve() error
	Stop()
	Error() error
}

type supService struct {
	Service
	fatalChan chan<- *FatalErr
}

func (s *supService) Serve() {
	err := s.Service.Serve()
	ferr := &FatalErr{}
	if errors.As(err, &ferr) {
		s.fatalChan <- ferr
	}
}

type Supervisor struct {
	sup         *suture.Supervisor
	spec        suture.Spec
	fatalChan   chan *FatalErr
	stopOnce    stdsync.Once
	services    map[ServiceToken]Service
	servicesMut stdsync.RWMutex
}

type Option func(*Supervisor)

func WithFailureRetry(threshold float64, backoff time.Duration) Option {
	return func(s *Supervisor) {
		s.spec.FailureThreshold = threshold
		s.spec.FailureBackoff = backoff
	}
}

func New(name string, opts ...Option) *Supervisor {
	s := &Supervisor{
		spec:      suture.Spec{PassThroughPanics: true},
		fatalChan: make(chan *FatalErr),
		stopOnce:  stdsync.Once{},
		services:  make(map[ServiceToken]Service),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.sup = suture.New(name, s.spec)
	return s
}

func (s *Supervisor) Serve() error {
	done := make(chan struct{})
	var fatalErr error
	go func() {
		for {
			select {
			case err := <-s.fatalChan:
				if fatalErr != nil {
					continue
				}
				fatalErr = err
				go s.Stop()
			case <-done:
				return
			}
		}
	}()
	s.sup.Serve()
	close(done)
	return fatalErr
}

func (s *Supervisor) Stop() {
	s.stopOnce.Do(s.sup.Stop)
}

func (s *Supervisor) Add(intf Service) ServiceToken {
	s.servicesMut.Lock()
	token := ServiceToken(s.sup.Add(&supService{
		Service:   intf,
		fatalChan: s.fatalChan,
	}))
	s.services[token] = intf
	s.servicesMut.Unlock()
	return token
}

func (s *Supervisor) Remove(token ServiceToken) {
	s.sup.Remove(suture.ServiceToken(token))
}

func (s *Supervisor) RemoveAndWait(token ServiceToken, d time.Duration) {
	s.sup.RemoveAndWait(suture.ServiceToken(token), d)
}

func (s *Supervisor) Services() []Service {
	s.servicesMut.RLock()
	services := make([]Service, 0, len(s.services))
	for _, s := range s.services {
		services = append(services, s)
	}
	s.servicesMut.RUnlock()
	return services
}

func (s *Supervisor) String() string {
	return s.sup.String()
}

func (s *Supervisor) Error() error {
	return nil
}

// AsService wraps the given function to implement Service by calling
// that function on serve and closing the passed channel when Stop is called.
func AsService(fn func(ctx context.Context) error, creator string) Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &service{
		serve:   fn,
		ctx:     ctx,
		cancel:  cancel,
		stopped: make(chan struct{}),
		creator: creator,
		mut:     sync.NewMutex(),
	}
	close(s.stopped) // not yet started, don't block on Stop()
	return s
}

type service struct {
	creator string
	serve   func(ctx context.Context) error
	ctx     context.Context
	cancel  context.CancelFunc
	stopped chan struct{}
	err     error
	mut     sync.Mutex
}

func (s *service) Serve() (err error) {
	s.mut.Lock()
	select {
	case <-s.ctx.Done():
		s.mut.Unlock()
		return
	default:
	}
	s.err = nil
	s.stopped = make(chan struct{})
	s.mut.Unlock()

	defer func() {
		if err == context.Canceled {
			err = nil
		}
		s.mut.Lock()
		s.err = err
		close(s.stopped)
		s.mut.Unlock()
	}()
	err = s.serve(s.ctx)
	return err
}

func (s *service) Stop() {
	s.mut.Lock()
	select {
	case <-s.ctx.Done():
		s.mut.Unlock()
		panic(fmt.Sprintf("Stop called more than once on %v", s))
	default:
		s.cancel()
	}

	// Cache s.stopped in a variable while we hold the mutex
	// to prevent a data race with Serve's resetting it.
	stopped := s.stopped
	s.mut.Unlock()
	<-stopped
}

func (s *service) Error() error {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.err
}

func (s *service) String() string {
	return fmt.Sprintf("Service@%p created by %v", s, s.creator)
}
