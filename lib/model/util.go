// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"sync"
	"time"
)

type Holdable interface {
	Holders() string
}

func newDeadlockDetector(timeout time.Duration) *deadlockDetector {
	return &deadlockDetector{
		timeout: timeout,
		lockers: make(map[string]sync.Locker),
	}
}

type deadlockDetector struct {
	timeout time.Duration
	lockers map[string]sync.Locker
}

func (d *deadlockDetector) Watch(name string, mut sync.Locker) {
	d.lockers[name] = mut
	go func() {
		for {
			time.Sleep(d.timeout / 4)
			ok := make(chan bool, 2)

			go func() {
				mut.Lock()
				_ = 1 // empty critical section
				mut.Unlock()
				ok <- true
			}()

			go func() {
				time.Sleep(d.timeout)
				ok <- false
			}()

			if r := <-ok; !r {
				msg := fmt.Sprintf("deadlock detected at %s", name)
				for otherName, otherMut := range d.lockers {
					if otherHolder, ok := otherMut.(Holdable); ok {
						msg += "\n===" + otherName + "===\n" + otherHolder.Holders()
					}
				}
				panic(msg)
			}
		}
	}()
}
