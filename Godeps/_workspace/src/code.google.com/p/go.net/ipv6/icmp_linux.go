// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

type sysICMPFilter struct {
	Data [8]uint32
}

func (f *sysICMPFilter) set(typ ICMPType, block bool) {
	if block {
		f.Data[typ>>5] |= 1 << (uint32(typ) & 31)
	} else {
		f.Data[typ>>5] &^= 1 << (uint32(typ) & 31)
	}
}

func (f *sysICMPFilter) setAll(block bool) {
	for i := range f.Data {
		if block {
			f.Data[i] = 1<<32 - 1
		} else {
			f.Data[i] = 0
		}
	}
}

func (f *sysICMPFilter) willBlock(typ ICMPType) bool {
	return f.Data[typ>>5]&(1<<(uint32(typ)&31)) != 0
}
