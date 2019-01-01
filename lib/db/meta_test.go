// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"math/bits"
	"sort"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestEachFlagBit(t *testing.T) {
	cases := []struct {
		flags      uint32
		iterations int
	}{
		{0, 0},
		{1<<0 | 1<<3, 2},
		{1 << 0, 1},
		{1 << 31, 1},
		{1<<10 | 1<<20 | 1<<30, 3},
	}

	for _, tc := range cases {
		var flags uint32
		iterations := 0

		eachFlagBit(tc.flags, func(f uint32) {
			iterations++
			flags |= f
			if bits.OnesCount32(f) != 1 {
				t.Error("expected exactly one bit to be set in every call")
			}
		})

		if flags != tc.flags {
			t.Errorf("expected 0x%x flags, got 0x%x", tc.flags, flags)
		}
		if iterations != tc.iterations {
			t.Errorf("expected %d iterations, got %d", tc.iterations, iterations)
		}
	}
}

func TestMetaDevices(t *testing.T) {
	d1 := protocol.DeviceID{1}
	d2 := protocol.DeviceID{2}
	meta := newMetadataTracker()

	meta.addFile(d1, protocol.FileInfo{Sequence: 1})
	meta.addFile(d1, protocol.FileInfo{Sequence: 2, LocalFlags: 1})
	meta.addFile(d2, protocol.FileInfo{Sequence: 1})
	meta.addFile(d2, protocol.FileInfo{Sequence: 2, LocalFlags: 2})
	meta.addFile(protocol.LocalDeviceID, protocol.FileInfo{Sequence: 1})

	// There are five device/flags combos
	if l := len(meta.counts.Counts); l < 5 {
		t.Error("expected at least five buckets, not", l)
	}

	// There are only two non-local devices
	devs := meta.devices()
	if l := len(devs); l != 2 {
		t.Fatal("expected two devices, not", l)
	}

	// Check that we got the two devices we expect
	sort.Slice(devs, func(a, b int) bool {
		return devs[a].Compare(devs[b]) == -1
	})
	if devs[0] != d1 {
		t.Error("first device should be d1")
	}
	if devs[1] != d2 {
		t.Error("second device should be d2")
	}
}

func TestMetaSequences(t *testing.T) {
	d1 := protocol.DeviceID{1}
	meta := newMetadataTracker()

	meta.addFile(d1, protocol.FileInfo{Sequence: 1})
	meta.addFile(d1, protocol.FileInfo{Sequence: 2, RawInvalid: true})
	meta.addFile(d1, protocol.FileInfo{Sequence: 3})
	meta.addFile(d1, protocol.FileInfo{Sequence: 4, RawInvalid: true})
	meta.addFile(protocol.LocalDeviceID, protocol.FileInfo{Sequence: 1})
	meta.addFile(protocol.LocalDeviceID, protocol.FileInfo{Sequence: 2})
	meta.addFile(protocol.LocalDeviceID, protocol.FileInfo{Sequence: 3, LocalFlags: 1})
	meta.addFile(protocol.LocalDeviceID, protocol.FileInfo{Sequence: 4, LocalFlags: 2})

	if seq := meta.Sequence(d1); seq != 4 {
		t.Error("sequence of first device should be 4, not", seq)
	}
	if seq := meta.Sequence(protocol.LocalDeviceID); seq != 4 {
		t.Error("sequence of first device should be 4, not", seq)
	}
}
