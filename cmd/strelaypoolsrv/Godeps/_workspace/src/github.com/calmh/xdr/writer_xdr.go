// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

// +build !ipdr

package xdr

func (w *Writer) WriteUint8(v uint8) (int, error) {
	return w.WriteUint32(uint32(v))
}

func (w *Writer) WriteUint16(v uint16) (int, error) {
	return w.WriteUint32(uint32(v))
}
