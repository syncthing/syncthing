// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package xdr

import (
	"errors"
	"io"
)

var ErrElementSizeExceeded = errors.New("element size exceeded")

type Reader struct {
	r   io.Reader
	err error
	b   [8]byte
	sb  []byte
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
	if r.sb == nil {
		r.sb = make([]byte, 64)
	} else {
		r.sb = r.sb[:cap(r.sb)]
	}
	r.sb = r.ReadBytesInto(r.sb)
	return string(r.sb)
}

func (r *Reader) ReadStringMax(max int) string {
	if r.sb == nil {
		r.sb = make([]byte, 64)
	} else {
		r.sb = r.sb[:cap(r.sb)]
	}
	r.sb = r.ReadBytesMaxInto(max, r.sb)
	return string(r.sb)
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
	if max > 0 && l > max {
		r.err = ErrElementSizeExceeded
		return nil
	}

	if l+pad(l) > len(dst) {
		dst = make([]byte, l+pad(l))
	} else {
		dst = dst[:l+pad(l)]
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

func (r *Reader) Error() error {
	if r.err == nil {
		return nil
	}
	return XDRError{"read", r.err}
}
