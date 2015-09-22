// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package xdr

import (
	"fmt"
	"io"
	"reflect"
	"unsafe"
)

type Reader struct {
	r   io.Reader
	err error
	b   [8]byte
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: r,
	}
}

func (r *Reader) ReadRaw(bs []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}

	var n int
	n, r.err = io.ReadFull(r.r, bs)
	return n, r.err
}

func (r *Reader) ReadString() string {
	return r.ReadStringMax(0)
}

func (r *Reader) ReadStringMax(max int) string {
	buf := r.ReadBytesMaxInto(max, nil)
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	sh := reflect.StringHeader{
		Data: bh.Data,
		Len:  bh.Len,
	}
	return *((*string)(unsafe.Pointer(&sh)))
}

func (r *Reader) ReadBytes() []byte {
	return r.ReadBytesInto(nil)
}

func (r *Reader) ReadBytesMax(max int) []byte {
	return r.ReadBytesMaxInto(max, nil)
}

func (r *Reader) ReadBytesInto(dst []byte) []byte {
	return r.ReadBytesMaxInto(0, dst)
}

func (r *Reader) ReadBytesMaxInto(max int, dst []byte) []byte {
	if r.err != nil {
		return nil
	}

	l := int(r.ReadUint32())
	if r.err != nil {
		return nil
	}
	if l < 0 || max > 0 && l > max {
		// l may be negative on 32 bit builds
		r.err = ElementSizeExceeded("bytes field", l, max)
		return nil
	}

	if fullLen := l + pad(l); fullLen > len(dst) {
		dst = make([]byte, fullLen)
	} else {
		dst = dst[:fullLen]
	}

	var n int
	n, r.err = io.ReadFull(r.r, dst)
	if r.err != nil {
		if debug {
			dl.Printf("rd bytes (%d): %v", len(dst), r.err)
		}
		return nil
	}

	if debug {
		if n > maxDebugBytes {
			dl.Printf("rd bytes (%d): %x...", len(dst), dst[:maxDebugBytes])
		} else {
			dl.Printf("rd bytes (%d): %x", len(dst), dst)
		}
	}
	return dst[:l]
}

func (r *Reader) ReadBool() bool {
	return r.ReadUint8() != 0
}

func (r *Reader) ReadUint32() uint32 {
	if r.err != nil {
		return 0
	}

	_, r.err = io.ReadFull(r.r, r.b[:4])
	if r.err != nil {
		if debug {
			dl.Printf("rd uint32: %v", r.err)
		}
		return 0
	}

	v := uint32(r.b[3]) | uint32(r.b[2])<<8 | uint32(r.b[1])<<16 | uint32(r.b[0])<<24

	if debug {
		dl.Printf("rd uint32=%d (0x%08x)", v, v)
	}
	return v
}

func (r *Reader) ReadUint64() uint64 {
	if r.err != nil {
		return 0
	}

	_, r.err = io.ReadFull(r.r, r.b[:8])
	if r.err != nil {
		if debug {
			dl.Printf("rd uint64: %v", r.err)
		}
		return 0
	}

	v := uint64(r.b[7]) | uint64(r.b[6])<<8 | uint64(r.b[5])<<16 | uint64(r.b[4])<<24 |
		uint64(r.b[3])<<32 | uint64(r.b[2])<<40 | uint64(r.b[1])<<48 | uint64(r.b[0])<<56

	if debug {
		dl.Printf("rd uint64=%d (0x%016x)", v, v)
	}
	return v
}

type XDRError struct {
	op  string
	err error
}

func (e XDRError) Error() string {
	return "xdr " + e.op + ": " + e.err.Error()
}

func (e XDRError) IsEOF() bool {
	return e.err == io.EOF
}

func (r *Reader) Error() error {
	if r.err == nil {
		return nil
	}
	return XDRError{"read", r.err}
}

func ElementSizeExceeded(field string, size, limit int) error {
	return fmt.Errorf("%s exceeds size limit; %d > %d", field, size, limit)
}
