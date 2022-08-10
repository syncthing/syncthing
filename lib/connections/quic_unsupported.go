// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build noquic || !go1.15
// +build noquic !go1.15

package connections

var errNotInBuild = fmt.Errorf("%w: disabled at build time", errUnsupported)

func init() {
	for _, scheme := range []string{"quic", "quic4", "quic6"} {
		listeners[scheme] = invalidListener{err: errNotInBuild}
		dialers[scheme] = invalidDialer{err: errNotInBuild}
	}
}
