// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serviceutil

import (
	"context"
	"fmt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture"
)

// AsService wraps the given function to implement suture.Service by calling
// that function on serve and closing the passed channel when Stop is called.
func AsService(fn func(ctx context.Context), creator string, opts ...Option) suture.Service {
	return asServiceWithError(func(ctx context.Context) error {
		fn(ctx)
		return nil
	}, creator, opts...)
}

type ServiceWithError interface {
	suture.Service
	fmt.Stringer
	Error() error
	SetError(error)
}

type Option interface {
	apply(*service)
}

type option struct {
	fn func(*service)
}

func (o *option) apply(s *service) {
	o.fn(s)
}

func WithConfigSubscription(w config.Wrapper, c config.Committer, initCfg func(config.Configuration)) Option {
	return &option{func(s *service) {
		oldServe := s.serve
		s.serve = func(ctx context.Context) error {
			cfg := w.Subscribe(c)
			initCfg(cfg)
			defer w.Unsubscribe(c)
			return oldServe(ctx)
		}
	}}
}

// AsServiceWithError does the same as AsService, except that it keeps track
// of an error returned by the given function.
func AsServiceWithError(fn func(ctx context.Context) error, creator string, opts ...Option) ServiceWithError {
	return asServiceWithError(fn, creator, opts...)
}

func asServiceWithError(fn func(ctx context.Context) error, creator string, opts ...Option) ServiceWithError {
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
	for _, opt := range opts {
		opt.apply(s)
	}
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

func (s *service) Serve() {
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

	var err error
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

func (s *service) SetError(err error) {
	s.mut.Lock()
	s.err = err
	s.mut.Unlock()
}

func (s *service) String() string {
	return fmt.Sprintf("Service@%p created by %v", s, s.creator)
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

func ConfigSubscriptionService(w config.Wrapper, c config.Committer, initCfg func(config.Configuration)) suture.Service {
	return AsService(func(ctx context.Context) { <-ctx.Done() }, c.String(), WithConfigSubscription(w, c, initCfg))
}
