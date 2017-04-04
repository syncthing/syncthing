// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package beacon

import (
	"net"
	stdsync "sync"

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

type errorHolder struct {
	err error
	mut stdsync.Mutex // uses stdlib sync as I want this to be trivially embeddable, and there is no risk of blocking
}

func (e *errorHolder) setError(err error) {
	e.mut.Lock()
	e.err = err
	e.mut.Unlock()
}

func (e *errorHolder) Error() error {
	e.mut.Lock()
	err := e.err
	e.mut.Unlock()
	return err
}
