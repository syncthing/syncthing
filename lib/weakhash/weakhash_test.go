// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// The existence of this file means we get 0% test coverage rather than no
// test coverage at all. Remove when implementing an actual test.

package weakhash

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"testing"
)

var payload = []byte("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz")

func TestFinder(t *testing.T) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.Write(payload); err != nil {
		t.Error(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Error(err)
	}

	hashes := []uint32{65143183, 65798547}
	finder, err := NewFinder(context.Background(), f, 4, hashes)
	if err != nil {
		t.Error(err)
	}

	expected := map[uint32][]int64{
		65143183: {1, 27, 53, 79},
		65798547: {2, 28, 54, 80},
	}
	actual := make(map[uint32][]int64)

	b := make([]byte, Size)

	for _, hash := range hashes {
		_, err := finder.Iterate(hash, b[:4], func(offset int64) bool {
			if !bytes.Equal(b, payload[offset:offset+4]) {
				t.Errorf("Not equal at %d: %s != %s", offset, string(b), string(payload[offset:offset+4]))
			}
			actual[hash] = append(actual[hash], offset)
			return true
		})
		if err != nil {
			t.Error(err)
		}
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Not equal: %#v != %#v", actual, expected)
	}
}
