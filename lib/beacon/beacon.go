// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"net"

	"github.com/thejerf/suture"
)

type recv struct {
	data []byte
	src  net.Addr
}

type Interface interface {
	suture.Service
	Send(data []byte)
	Recv() ([]byte, net.Addr)
	Error() error
}
