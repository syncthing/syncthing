// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"sync"
	"testing"
)

func TestControlFlags(t *testing.T) {
	tf := FlagInterface | FlagPathMTU
	opt := rawOpt{cflags: tf | FlagHopLimit}

	// This loop runs methods of raw.Opt concurrently for testing
	// concurrent access to the rawOpt. The first entry shold be
	// opt.set and the last entry should be opt.clear.
	tfns := []func(ControlFlags){opt.set, opt.clear, opt.clear}
	ch := make(chan bool)
	var wg sync.WaitGroup
	for i, fn := range tfns {
		wg.Add(1)
		go func(i int, fn func(ControlFlags)) {
			defer wg.Done()
			switch i {
			case 0:
				close(ch)
			case len(tfns) - 1:
				<-ch
			}
			opt.Lock()
			defer opt.Unlock()
			fn(tf)
		}(i, fn)
	}
	wg.Wait()

	if opt.isset(tf) {
		t.Fatalf("got %#x; expected %#x", opt.cflags, FlagHopLimit)
	}
}
