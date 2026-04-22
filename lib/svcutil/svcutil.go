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
	"log/slog"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/slogutil"
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

func (*FatalErr) Is(target error) bool {
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

func (*noRestartErr) Is(target error) bool {
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

	// The error returned by serve() may well be a network timeout, which as
	// of Go 1.19 is a context.DeadlineExceeded, which Suture interprets as
	// a signal to stop the service instead of restarting it. This typically
	// isn't what we want, so we make sure to remove the context specific
	// error types unless *our* context is actually cancelled.
	err := asNonContextError(ctx, s.serve(ctx))

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

func SpecWithDebugLogger() suture.Spec {
	return spec(func(e suture.Event) { slog.Debug(e.String()) })
}

func SpecWithInfoLogger() suture.Spec {
	return spec(infoEventHook())
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
func infoEventHook() suture.EventHook {
	var prevTerminate suture.EventServiceTerminate
	return func(ei suture.Event) {
		m := ei.Map()
		l := slog.Default().With("supervisor", m["supervisor_name"], "service", m["service_name"])
		switch e := ei.(type) {
		case suture.EventStopTimeout:
			l.Warn("Service failed to terminate in a timely manner")
		case suture.EventServicePanic:
			l.Error("Caught a service panic, which shouldn't happen")
			l.Warn(e.String()) //nolint:sloglint
		case suture.EventServiceTerminate:
			if e.ServiceName == prevTerminate.ServiceName && e.Err == prevTerminate.Err {
				l.Debug("Service failed repeatedly", slogutil.Error(e.Err))
			} else {
				l.Warn("Service failed", slogutil.Error(e.Err))
			}
			prevTerminate = e
			l.Debug(e.String()) // Contains some backoff statistics
		case suture.EventBackoff:
			l.Debug("Exiting the backoff state")
		case suture.EventResume:
			l.Debug("Too many service failures - entering the backoff state")
		default:
			l.Warn("Unknown suture supervisor event", slog.Any("type", e.Type()))
			l.Warn(e.String()) //nolint:sloglint
		}
	}
}

// asNonContextError returns err, except if it is context.Canceled or
// context.DeadlineExceeded in which case the error will be a simple string
// representation instead. The given context is checked for cancellation,
// and if it is cancelled then that error is returned instead of err.
func asNonContextError(ctx context.Context, err error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s (non-context)", err.Error())
	}
	return err
}

func CallWithContext(ctx context.Context, fn func() error) error {
	var err error
	done := make(chan struct{})
	go func() {
		err = fn()
		close(done)
	}()
	select {
	case <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
