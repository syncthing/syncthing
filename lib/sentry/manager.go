// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"sync/atomic"

	"github.com/getsentry/raven-go"
)

var Manager = &RavenManager{
	enabled:            0,
	configDecisionMade: make(chan struct{}),
}

type RavenManager struct {
	enabled            int32
	configDecisionMade chan struct{}
}

func (t *RavenManager) SetDSN(dsn string) {
	atomic.StoreInt32(&t.enabled, int32(len(dsn)))
	_ = raven.SetDSN(dsn)

	select {
	case <-t.configDecisionMade:
	default:
		close(t.configDecisionMade)
	}
}

func (t *RavenManager) String() string {
	return "RavenManager"
}

func (t *RavenManager) isSentryEnabled() bool {
	// Block for the initial decision to be made
	<-t.configDecisionMade

	return atomic.LoadInt32(&t.enabled) > 0
}
