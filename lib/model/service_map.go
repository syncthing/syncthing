// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/thejerf/suture/v4"
)

var errSvcNotFound = errors.New("service not found")

// A serviceMap is a utility map of arbitrary keys to a suture.Service of
// some kind, where adding and removing services ensures they are properly
// started and stopped on the given Supervisor. The serviceMap is itself a
// suture.Service and should be added to a Supervisor.
// Not safe for concurrent use.
type serviceMap[K comparable, S suture.Service] struct {
	services    map[K]S
	tokens      map[K]suture.ServiceToken
	supervisor  *suture.Supervisor
	eventLogger events.Logger
}

func newServiceMap[K comparable, S suture.Service](eventLogger events.Logger) *serviceMap[K, S] {
	m := &serviceMap[K, S]{
		services:    make(map[K]S),
		tokens:      make(map[K]suture.ServiceToken),
		eventLogger: eventLogger,
	}
	m.supervisor = suture.New(m.String(), svcutil.SpecWithDebugLogger())
	return m
}

// Add adds a service to the map, starting it on the supervisor. If there is
// already a service at the given key, it is removed first.
func (s *serviceMap[K, S]) Add(k K, v S) {
	if tok, ok := s.tokens[k]; ok {
		// There is already a service at this key, remove it first.
		s.supervisor.Remove(tok)
	}
	s.services[k] = v
	s.tokens[k] = s.supervisor.Add(v)
}

// Get returns the service at the given key, or the empty value and false if
// there is no service at that key.
func (s *serviceMap[K, S]) Get(k K) (v S, ok bool) {
	v, ok = s.services[k]
	return
}

// Stop removes the service at the given key from the supervisor, stopping it.
// The service itself is still retained, i.e. a call to Get with the same key
// will still return a result.
func (s *serviceMap[K, S]) Stop(k K) {
	if tok, ok := s.tokens[k]; ok {
		s.supervisor.Remove(tok)
	}
}

// StopAndWaitChan removes the service at the given key from the supervisor,
// stopping it. The service itself is still retained, i.e. a call to Get with
// the same key will still return a result.
// The returned channel will produce precisely one error value: either the
// return value from RemoveAndWait (possibly nil), or errSvcNotFound if the
// service was not found.
func (s *serviceMap[K, S]) StopAndWaitChan(k K, timeout time.Duration) <-chan error {
	ret := make(chan error, 1)
	if tok, ok := s.tokens[k]; ok {
		go func() {
			ret <- s.supervisor.RemoveAndWait(tok, timeout)
		}()
	} else {
		ret <- errSvcNotFound
	}
	return ret
}

// Remove removes the service at the given key, stopping it on the supervisor.
// If there is no service at the given key, nothing happens. The return value
// indicates whether a service was removed.
func (s *serviceMap[K, S]) Remove(k K) (found bool) {
	if tok, ok := s.tokens[k]; ok {
		found = true
		s.supervisor.Remove(tok)
	} else {
		_, found = s.services[k]
	}
	delete(s.services, k)
	delete(s.tokens, k)
	return
}

// RemoveAndWait removes the service at the given key, stopping it on the
// supervisor. Returns errSvcNotFound if there is no service at the given
// key, otherwise the return value from the supervisor's RemoveAndWait.
func (s *serviceMap[K, S]) RemoveAndWait(k K, timeout time.Duration) error {
	return <-s.RemoveAndWaitChan(k, timeout)
}

// RemoveAndWaitChan removes the service at the given key, stopping it on
// the supervisor. The returned channel will produce precisely one error
// value: either the return value from RemoveAndWait (possibly nil), or
// errSvcNotFound if the service was not found.
func (s *serviceMap[K, S]) RemoveAndWaitChan(k K, timeout time.Duration) <-chan error {
	ret := s.StopAndWaitChan(k, timeout)
	delete(s.services, k)
	return ret
}

// Each calls the given function for each service in the map. An error from
// fn will stop the iteration and be returned as-is.
func (s *serviceMap[K, S]) Each(fn func(K, S) error) error {
	for key, svc := range s.services {
		if err := fn(key, svc); err != nil {
			return err
		}
	}
	return nil
}

// Suture implementation

func (s *serviceMap[K, S]) Serve(ctx context.Context) error {
	return s.supervisor.Serve(ctx)
}

func (s *serviceMap[K, S]) String() string {
	var kv K
	var sv S
	return fmt.Sprintf("serviceMap[%T, %T]@%p", kv, sv, s)
}
