// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

type rawICMPFilter struct {
	// TODO(mikio): Implement this
}

func (f *rawICMPFilter) set(typ ICMPType, block bool) {
	// TODO(mikio): Implement this
}

func (f *rawICMPFilter) setAll(block bool) {
	// TODO(mikio): Implement this
}

func (f *rawICMPFilter) willBlock(typ ICMPType) bool {
	// TODO(mikio): Implement this
	return false
}
