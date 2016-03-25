// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/tls"
	"time"
)

type mockedRelayService struct{}

// from suture.Service

func (s *mockedRelayService) Serve() {
	select {}
}

func (s *mockedRelayService) Stop() {
}

// from relay.Service

func (s *mockedRelayService) Accept() *tls.Conn {
	return nil
}

func (s *mockedRelayService) Relays() []string {
	return nil
}

func (s *mockedRelayService) RelayStatus(uri string) (time.Duration, bool) {
	return 0, false
}
