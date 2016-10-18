// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"sync"
	"time"
)

type Holder interface {
	Holder() (string, int)
}

func deadlockDetect(mut sync.Locker, timeout time.Duration, name string) {
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
				msg := fmt.Sprintf("deadlock detected at %s", name)
				if hmut, ok := mut.(Holder); ok {
					holder, goid := hmut.Holder()
					msg = fmt.Sprintf("deadlock detected at %s, current holder: %s at routine %d", name, holder, goid)
				}
				panic(msg)
			}
		}
	}()
}
