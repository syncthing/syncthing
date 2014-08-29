// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ipdr

package xdr

import "io"

func (r *Reader) ReadUint8() uint8 {
	if r.err != nil {
		return 0
	}

	_, r.err = io.ReadFull(r.r, r.b[:1])
	if r.err != nil {
		if debug {
			dl.Printf("rd uint8: %v", r.err)
		}
		return 0
	}

	if debug {
		dl.Printf("rd uint8=%d (0x%02x)", r.b[0], r.b[0])
	}
	return r.b[0]
}

func (r *Reader) ReadUint16() uint16 {
	if r.err != nil {
		return 0
	}

	_, r.err = io.ReadFull(r.r, r.b[:2])
	if r.err != nil {
		if debug {
			dl.Printf("rd uint16: %v", r.err)
		}
		return 0
	}

	v := uint16(r.b[1]) | uint16(r.b[0])<<8

	if debug {
		dl.Printf("rd uint16=%d (0x%04x)", v, v)
	}
	return v
}
