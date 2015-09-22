// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr

import (
	"bytes"
	"strings"
	"testing"
	"testing/quick"
)

func TestBytesNil(t *testing.T) {
	fn := func(bs []byte) bool {
		var b = new(bytes.Buffer)
		var w = NewWriter(b)
		var r = NewReader(b)
		w.WriteBytes(bs)
		w.WriteBytes(bs)
		r.ReadBytes()
		res := r.ReadBytes()
		return bytes.Compare(bs, res) == 0
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestBytesGiven(t *testing.T) {
	fn := func(bs []byte) bool {
		var b = new(bytes.Buffer)
		var w = NewWriter(b)
		var r = NewReader(b)
		w.WriteBytes(bs)
		w.WriteBytes(bs)
		res := make([]byte, 12)
		res = r.ReadBytesInto(res)
		res = r.ReadBytesInto(res)
		return bytes.Compare(bs, res) == 0
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestReadBytesMaxInto(t *testing.T) {
	var max = 64
	for tot := 32; tot < 128; tot++ {
		for diff := -32; diff <= 32; diff++ {
			var b = new(bytes.Buffer)
			var r = NewReader(b)
			var w = NewWriter(b)

			var toWrite = make([]byte, tot)
			w.WriteBytes(toWrite)

			var buf = make([]byte, tot+diff)
			var bs = r.ReadBytesMaxInto(max, buf)

			if tot <= max {
				if read := len(bs); read != tot {
					t.Errorf("Incorrect read bytes, wrote=%d, buf=%d, max=%d, read=%d", tot, tot+diff, max, read)
				}
			} else if !strings.Contains(r.err.Error(), "exceeds size") {
				t.Errorf("Unexpected non-ErrElementSizeExceeded error for wrote=%d, max=%d: %v", tot, max, r.err)
			}
		}
	}
}

func TestReadStringMax(t *testing.T) {
	for tot := 42; tot < 72; tot++ {
		for max := 0; max < 128; max++ {
			var b = new(bytes.Buffer)
			var r = NewReader(b)
			var w = NewWriter(b)

			var toWrite = make([]byte, tot)
			w.WriteBytes(toWrite)

			var str = r.ReadStringMax(max)
			var read = len(str)

			if max == 0 || tot <= max {
				if read != tot {
					t.Errorf("Incorrect read bytes, wrote=%d, max=%d, read=%d", tot, max, read)
				}
			} else if !strings.Contains(r.err.Error(), "exceeds size") {
				t.Errorf("Unexpected non-ErrElementSizeExceeded error for wrote=%d, max=%d, read=%d: %v", tot, max, read, r.err)
			}
		}
	}
}
