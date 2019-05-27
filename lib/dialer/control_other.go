// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !aix,!darwin,!dragonfly,!freebsd,!linux,!netbsd,!openbsd,!windows

package dialer

var SupportsReusePort = false

func ReusePortControl(network, address string, c syscall.RawConn) error {
	return nil
}
