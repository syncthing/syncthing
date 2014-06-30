// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package xdr_test

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o XDRBenchStruct) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o XDRBenchStruct) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o XDRBenchStruct) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint64(o.I1)
	xw.WriteUint32(o.I2)
	xw.WriteUint16(o.I3)
	xw.WriteBytes(o.Bs)
	xw.WriteString(o.S)
	return xw.Tot(), xw.Error()
}

func (o *XDRBenchStruct) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *XDRBenchStruct) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *XDRBenchStruct) decodeXDR(xr *xdr.Reader) error {
	o.I1 = xr.ReadUint64()
	o.I2 = xr.ReadUint32()
	o.I3 = xr.ReadUint16()
	o.Bs = xr.ReadBytes()
	o.S = xr.ReadString()
	return xr.Error()
}
