// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import "testing"

func TestZeroByteSempahore(t *testing.T) {
	// A semaphore with zero capacity is just a no-op.

	s := newByteSemaphore(0)

	// None of these should block or panic
	s.take(123)
	s.take(456)
	s.give(1 << 30)
}

func TestByteSempahoreCapChange(t *testing.T) {
	// Waiting takes should unblock when the capacity increases

	s := newByteSemaphore(100)

	s.take(75)

	gotit := make(chan struct{})
	go func() {
		s.take(75)
		close(gotit)
	}()

	s.setCapacity(150)
	<-gotit
}
