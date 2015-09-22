// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

// +build ipdr

package xdr

func (w *Writer) WriteUint8(v uint8) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if debug {
		dl.Printf("wr uint8=%d", v)
	}

	w.b[0] = byte(v)

	var l int
	l, w.err = w.w.Write(w.b[:1])
	w.tot += l
	return l, w.err
}

func (w *Writer) WriteUint16(v uint16) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if debug {
		dl.Printf("wr uint8=%d", v)
	}

	w.b[0] = byte(v >> 8)
	w.b[1] = byte(v)

	var l int
	l, w.err = w.w.Write(w.b[:2])
	w.tot += l
	return l, w.err
}
