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
	"io"
)

type ReplacingWriter struct {
	Writer io.Writer
	From   byte
	To     []byte
}

func (w ReplacingWriter) Write(bs []byte) (int, error) {
	var n, written int
	var err error

	newlineIdx := bytes.IndexByte(bs, w.From)
	for newlineIdx >= 0 {
		n, err = w.Writer.Write(bs[:newlineIdx])
		written += n
		if err != nil {
			break
		}
		if len(w.To) > 0 {
			n, err := w.Writer.Write(w.To)
			if n == len(w.To) {
				written++
			}
			if err != nil {
				break
			}
		}
		bs = bs[newlineIdx+1:]
		newlineIdx = bytes.IndexByte(bs, w.From)
	}

	n, err = w.Writer.Write(bs)
	written += n

	return written, err
}
