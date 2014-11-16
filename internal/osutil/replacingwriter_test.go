// Copyright (C) 2014 The Syncthing Authors.
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
