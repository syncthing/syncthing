// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"bytes"
	"fmt"
	"testing"
)

var testcases = []struct {
	from byte
	to   []byte
	a, b string
}{
	{'\n', []byte{'\r', '\n'}, "", ""},
	{'\n', []byte{'\r', '\n'}, "foo", "foo"},
	{'\n', []byte{'\r', '\n'}, "foo\n", "foo\r\n"},
	{'\n', []byte{'\r', '\n'}, "foo\nbar", "foo\r\nbar"},
	{'\n', []byte{'\r', '\n'}, "foo\nbar\nbaz", "foo\r\nbar\r\nbaz"},
	{'\n', []byte{'\r', '\n'}, "\nbar", "\r\nbar"},
	{'o', []byte{'x', 'l', 'r'}, "\nfoo", "\nfxlrxlr"},
	{'o', nil, "\nfoo", "\nf"},
	{'f', []byte{}, "\nfoo", "\noo"},
}

func TestReplacingWriter(t *testing.T) {
	for _, tc := range testcases {
		var buf bytes.Buffer
		w := ReplacingWriter{
			Writer: &buf,
			From:   tc.from,
			To:     tc.to,
		}
		fmt.Fprint(w, tc.a)
		if buf.String() != tc.b {
			t.Errorf("%q != %q", buf.String(), tc.b)
		}
	}
}
