// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build !ipdr

package xdr

func (r *Reader) ReadUint8() uint8 {
	return uint8(r.ReadUint32())
}

func (r *Reader) ReadUint16() uint16 {
	return uint16(r.ReadUint32())
}
