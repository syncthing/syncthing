// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"github.com/getsentry/raven-go"
	"github.com/pkg/errors"
)

var ErrSentryDisabled = errors.New("sentry disabled via config")

type managerAwareTransportWrapper struct {
	underlying raven.Transport
}

func (t *managerAwareTransportWrapper) Send(url, authHeader string, packet *raven.Packet) error {
	if Manager.isSentryEnabled() {
		return t.underlying.Send(url, authHeader, packet)
	}
	return ErrSentryDisabled
}
