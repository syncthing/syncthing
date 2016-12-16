// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// The existence of this file means we get 0% test coverage rather than no
// test coverage at all. Remove when implementing an actual test.

package weakhash

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

var payload = []byte("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz")
var hashes = []uint32{
	64225674,
	64881038,
	65536402,
	66191766,
	66847130,
	67502494,
	68157858,
	68813222,
	69468586,
	70123950,
	70779314,
	71434678,
	72090042,
	72745406,
	73400770,
	74056134,
	74711498,
	75366862,
	76022226,
	76677590,
	77332954,
	77988318,
	78643682,
	77595084,
	74842550,
	70386080,
	64225674,
	64881038,
	65536402,
	66191766,
	66847130,
	67502494,
	68157858,
	68813222,
	69468586,
	70123950,
	70779314,
	71434678,
	72090042,
	72745406,
	73400770,
	74056134,
	74711498,
	75366862,
	76022226,
	76677590,
	77332954,
	77988318,
	78643682,
	77595084,
	74842550,
	70386080,
	64225674,
	64881038,
	65536402,
	66191766,
	66847130,
	67502494,
	68157858,
	68813222,
	69468586,
	70123950,
	70779314,
	71434678,
	72090042,
	72745406,
	73400770,
	74056134,
	74711498,
	75366862,
	76022226,
	76677590,
	77332954,
	77988318,
	78643682,
	77595084,
	74842550,
	70386080,
	64225674,
	64881038,
	65536402,
	66191766,
	66847130,
	67502494,
	68157858,
	68813222,
	69468586,
	70123950,
	70779314,
	71434678,
	72090042,
	72745406,
	73400770,
	74056134,
	74711498,
	75366862,
	76022226,
	76677590,
	77332954,
	77988318,
	78643682,
	71893365,
	71893365,
}

// Tested using an alternative C implementation at https://gist.github.com/csabahenk/1096262
func TestHashCorrect(t *testing.T) {
	h := NewHash(Size)
	pos := 0
	for pos < Size {
		h.Write([]byte{payload[pos]})
		pos++
	}

	for i := 0; pos < len(payload); i++ {
		if h.Sum32() != hashes[i] {
			t.Errorf("mismatch at %d", i)
		}
		h.Write([]byte{payload[pos]})
		pos++
	}
}

func TestFinder(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.Write(payload); err != nil {
		t.Error(err)
	}

	hashes := []uint32{64881038, 65536402}
	finder, err := NewFinder(f.Name(), 4, hashes)
	if err != nil {
		t.Error(err)
	}
	defer finder.Close()

	expected := map[uint32][]int64{
		64881038: []int64{1, 27, 53, 79},
		65536402: []int64{2, 28, 54, 80},
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
