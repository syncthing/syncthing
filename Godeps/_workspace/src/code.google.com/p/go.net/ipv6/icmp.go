// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import "sync"

// An ICMPType represents a type of ICMP message.
type ICMPType int

func (typ ICMPType) String() string {
	s, ok := icmpTypes[typ]
	if !ok {
		return "<nil>"
	}
	return s
}

// An ICMPFilter represents an ICMP message filter for incoming
// packets.
type ICMPFilter struct {
	mu sync.RWMutex
	rawICMPFilter
}

// Set sets the ICMP type and filter action to the filter.
func (f *ICMPFilter) Set(typ ICMPType, block bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set(typ, block)
}

// SetAll sets the filter action to the filter.
func (f *ICMPFilter) SetAll(block bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setAll(block)
}

// WillBlock reports whether the ICMP type will be blocked.
func (f *ICMPFilter) WillBlock(typ ICMPType) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.willBlock(typ)
}
