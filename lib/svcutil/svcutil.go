// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package svcutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture/v4"
)

const ServiceTimeout = 10 * time.Second

type FatalErr struct {
	Err    error
	Status ExitStatus
}

// AsFatalErr wraps the given error creating a FatalErr. If the given error
// already is of type FatalErr, it is not wrapped again.
func AsFatalErr(err error, status ExitStatus) *FatalErr {
	var ferr *FatalErr
	if errors.As(err, &ferr) {
		return ferr
	}
	return &FatalErr{
		Err:    err,
		Status: status,
	}
}

func IsFatal(err error) bool {
	ferr := &FatalErr{}
	return errors.As(err, &ferr)
}

func (e *FatalErr) Error() string {
	return e.Err.Error()
}

func (e *FatalErr) Unwrap() error {
	return e.Err
}

func (e *FatalErr) Is(target error) bool {
	return target == suture.ErrTerminateSupervisorTree
}

// NoRestartErr wraps the given error err (which may be nil) to make sure that
// `errors.Is(err, suture.ErrDoNotRestart) == true`.
func NoRestartErr(err error) error {
	if err == nil {
		return suture.ErrDoNotRestart
	}
	return &noRestartErr{err}
}

type noRestartErr struct {
	err error
}

func (e *noRestartErr) Error() string {
	return e.err.Error()
}

func (e *noRestartErr) Unwrap() error {
	return e.err
}

func (e *noRestartErr) Is(target error) bool {
	return target == suture.ErrDoNotRestart
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

type ServiceWithError interface {
	suture.Service
	fmt.Stringer
	Error() error
}

// AsService wraps the given function to implement suture.Service. In addition
// it keeps track of the returned error and allows querying that error.
func AsService(fn func(ctx context.Context) error, creator string) ServiceWithError {
	return &service{
		creator: creator,
		serve:   fn,
		mut:     sync.NewMutex(),
	}
}

type service struct {
	creator string
	serve   func(ctx context.Context) error
	err     error
	mut     sync.Mutex
}

func (s *service) Serve(ctx context.Context) error {
	s.mut.Lock()
	s.err = nil
	s.mut.Unlock()

	err := s.serve(ctx)

	s.mut.Lock()
	s.err = err
	s.mut.Unlock()

	return err
}

func (s *service) Error() error {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.err
}

func (s *service) String() string {
	return fmt.Sprintf("Service@%p created by %v", s, s.creator)

}

type doneService func()

func (fn doneService) Serve(ctx context.Context) error {
	<-ctx.Done()
	fn()
	return nil
}

// OnSupervisorDone calls fn when sup is done.
func OnSupervisorDone(sup *suture.Supervisor, fn func()) {
	sup.Add(doneService(fn))
}

func SpecWithDebugLogger(l logger.Logger) suture.Spec {
	return spec(func(e suture.Event) { l.Debugln(e) })
}

func SpecWithInfoLogger(l logger.Logger) suture.Spec {
	return spec(infoEventHook(l))
}

func spec(eventHook suture.EventHook) suture.Spec {
	return suture.Spec{
		EventHook:                eventHook,
		Timeout:                  ServiceTimeout,
		PassThroughPanics:        true,
		DontPropagateTermination: false,
	}
}

// infoEventHook prints service failures and failures to stop services at level
// info. All other events and identical, consecutive failures are logged at
// debug only.
func infoEventHook(l logger.Logger) suture.EventHook {
	var prevTerminate suture.EventServiceTerminate
	return func(ei suture.Event) {
		switch e := ei.(type) {
		case suture.EventStopTimeout:
			l.Infof("%s: Service %s failed to terminate in a timely manner", e.SupervisorName, e.ServiceName)
		case suture.EventServicePanic:
			l.Warnln("Caught a service panic, which shouldn't happen")
			l.Infoln(e)
		case suture.EventServiceTerminate:
			msg := fmt.Sprintf("%s: service %s failed: %s", e.SupervisorName, e.ServiceName, e.Err)
			if e.ServiceName == prevTerminate.ServiceName && e.Err == prevTerminate.Err {
				l.Debugln(msg)
			} else {
				l.Infoln(msg)
			}
			prevTerminate = e
			l.Debugln(e) // Contains some backoff statistics
		case suture.EventBackoff:
			l.Debugf("%s: exiting the backoff state.", e.SupervisorName)
		case suture.EventResume:
			l.Debugf("%s: too many service failures - entering the backoff state.", e.SupervisorName)
		default:
			l.Warnln("Unknown suture supervisor event type", e.Type())
			l.Infoln(e)
		}
	}
}
