// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"time"

	"github.com/thejerf/suture"
)

type Service interface {
	Serve()
	Stop()
}

type ServiceToken struct {
	token suture.ServiceToken
}

type Supervisor struct {
	s *suture.Supervisor
}

func (s *Supervisor) Serve() {
	defer ReportPanic()
	s.s.Serve()
}

func (s *Supervisor) ServeBackground() {
	go s.Serve()
}

func (s *Supervisor) Add(svc Service) ServiceToken {
	token := s.s.Add(&serviceWrapper{svc})
	return ServiceToken{token}
}

func (s *Supervisor) Remove(token ServiceToken) error {
	return s.s.Remove(token.token)
}

func (s *Supervisor) RemoveAndWait(token ServiceToken, timeout time.Duration) error {
	return s.s.RemoveAndWait(token.token, timeout)
}

func (s *Supervisor) String() string {
	return s.s.String()
}

func (s *Supervisor) Stop() {
	s.s.Stop()
}

type Spec = suture.Spec

func NewSupervisor(name string, spec Spec) *Supervisor {
	return &Supervisor{suture.New(name, spec)}
}

type serviceWrapper struct {
	Service
}

func (w *serviceWrapper) Serve() {
	defer ReportPanic()
	w.Service.Serve()
}
