// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
