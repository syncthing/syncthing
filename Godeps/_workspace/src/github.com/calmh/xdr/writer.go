// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr

import (
	"io"
	"reflect"
	"unsafe"
)

var padBytes = []byte{0, 0, 0}

type Writer struct {
	w   io.Writer
	tot int
	err error
	b   [8]byte
}

type AppendWriter []byte

func (w *AppendWriter) Write(bs []byte) (int, error) {
	*w = append(*w, bs...)
	return len(bs), nil
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

func (w *Writer) WriteRaw(bs []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	var n int
	n, w.err = w.w.Write(bs)
	return n, w.err
}

func (w *Writer) WriteString(s string) (int, error) {
	sh := *((*reflect.StringHeader)(unsafe.Pointer(&s)))
	bh := reflect.SliceHeader{
		Data: sh.Data,
		Len:  sh.Len,
		Cap:  sh.Len,
	}
	return w.WriteBytes(*(*[]byte)(unsafe.Pointer(&bh)))
}

func (w *Writer) WriteBytes(bs []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	w.WriteUint32(uint32(len(bs)))
	if w.err != nil {
		return 0, w.err
	}

	if debug {
		if len(bs) > maxDebugBytes {
			dl.Printf("wr bytes (%d): %x...", len(bs), bs[:maxDebugBytes])
		} else {
			dl.Printf("wr bytes (%d): %x", len(bs), bs)
		}
	}

	var l, n int
	n, w.err = w.w.Write(bs)
	l += n

	if p := pad(len(bs)); w.err == nil && p > 0 {
		n, w.err = w.w.Write(padBytes[:p])
		l += n
	}

	w.tot += l
	return l, w.err
}

func (w *Writer) WriteBool(v bool) (int, error) {
	if v {
		return w.WriteUint8(1)
	} else {
		return w.WriteUint8(0)
	}
}

func (w *Writer) WriteUint32(v uint32) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if debug {
		dl.Printf("wr uint32=%d", v)
	}

	w.b[0] = byte(v >> 24)
	w.b[1] = byte(v >> 16)
	w.b[2] = byte(v >> 8)
	w.b[3] = byte(v)

	var l int
	l, w.err = w.w.Write(w.b[:4])
	w.tot += l
	return l, w.err
}

func (w *Writer) WriteUint64(v uint64) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	if debug {
		dl.Printf("wr uint64=%d", v)
	}

	w.b[0] = byte(v >> 56)
	w.b[1] = byte(v >> 48)
	w.b[2] = byte(v >> 40)
	w.b[3] = byte(v >> 32)
	w.b[4] = byte(v >> 24)
	w.b[5] = byte(v >> 16)
	w.b[6] = byte(v >> 8)
	w.b[7] = byte(v)

	var l int
	l, w.err = w.w.Write(w.b[:8])
	w.tot += l
	return l, w.err
}

func (w *Writer) Tot() int {
	return w.tot
}

func (w *Writer) Error() error {
	if w.err == nil {
		return nil
	}
	return XDRError{"write", w.err}
}
