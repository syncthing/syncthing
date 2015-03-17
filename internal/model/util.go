// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"sync"
	"time"
)

func deadlockDetect(mut sync.Locker, timeout time.Duration) {
	go func() {
		for {
			time.Sleep(timeout / 4)
			ok := make(chan bool, 2)

			go func() {
				mut.Lock()
				mut.Unlock()
				ok <- true
			}()

			go func() {
				time.Sleep(timeout)
				ok <- false
			}()

			if r := <-ok; !r {
				panic("deadlock detected")
			}
		}
	}()
}
