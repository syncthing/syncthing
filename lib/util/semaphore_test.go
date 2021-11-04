// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package util

import "testing"

func TestZeroByteSempahore(t *testing.T) {
	// A semaphore with zero capacity is just a no-op.

	s := NewSemaphore(0)

	// None of these should block or panic
	s.Take(123)
	s.Take(456)
	s.Give(1 << 30)
}

func TestByteSempahoreCapChangeUp(t *testing.T) {
	// Waiting takes should unblock when the capacity increases

	s := NewSemaphore(100)

	s.Take(75)
	if s.available != 25 {
		t.Error("bad state after take")
	}

	gotit := make(chan struct{})
	go func() {
		s.Take(75)
		close(gotit)
	}()

	s.SetCapacity(155)
	<-gotit
	if s.available != 5 {
		t.Error("bad state after both takes")
	}
}

func TestByteSempahoreCapChangeDown1(t *testing.T) {
	// Things should make sense when capacity is adjusted down

	s := NewSemaphore(100)

	s.Take(75)
	if s.available != 25 {
		t.Error("bad state after take")
	}

	s.SetCapacity(90)
	if s.available != 15 {
		t.Error("bad state after adjust")
	}

	s.Give(75)
	if s.available != 90 {
		t.Error("bad state after give")
	}
}

func TestByteSempahoreCapChangeDown2(t *testing.T) {
	// Things should make sense when capacity is adjusted down, different case

	s := NewSemaphore(100)

	s.Take(75)
	if s.available != 25 {
		t.Error("bad state after take")
	}

	s.SetCapacity(10)
	if s.available != 0 {
		t.Error("bad state after adjust")
	}

	s.Give(75)
	if s.available != 10 {
		t.Error("bad state after give")
	}
}

func TestByteSempahoreGiveMore(t *testing.T) {
	// We shouldn't end up with more available than we have capacity...

	s := NewSemaphore(100)

	s.Take(150)
	if s.available != 0 {
		t.Errorf("bad state after large take")
	}

	s.Give(150)
	if s.available != 100 {
		t.Errorf("bad state after large take + give")
	}

	s.Take(150)
	s.SetCapacity(125)
	// available was zero before, we're increasing capacity by 25
	if s.available != 25 {
		t.Errorf("bad state after setcap")
	}

	s.Give(150)
	if s.available != 125 {
		t.Errorf("bad state after large take + give with adjustment")
	}
}
