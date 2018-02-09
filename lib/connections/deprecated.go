// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import "github.com/syncthing/syncthing/lib/config"

// deprecatedListener is never valid
type deprecatedListener struct {
	listenerFactory
}

func (deprecatedListener) Valid(_ config.Configuration) error {
	return errDeprecated
}

// deprecatedDialer is never valid
type deprecatedDialer struct {
	dialerFactory
}

func (deprecatedDialer) Valid(_ config.Configuration) error {
	return errDeprecated
}

func init() {
	listeners["kcp"] = deprecatedListener{}
	listeners["kcp4"] = deprecatedListener{}
	listeners["kcp6"] = deprecatedListener{}
	dialers["kcp"] = deprecatedDialer{}
	dialers["kcp4"] = deprecatedDialer{}
	dialers["kcp6"] = deprecatedDialer{}
}
