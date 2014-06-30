package xdr_test

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o TestStruct) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o TestStruct) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o TestStruct) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint64(uint64(o.I))
	xw.WriteUint16(uint16(o.I16))
	xw.WriteUint16(o.UI16)
	xw.WriteUint32(uint32(o.I32))
	xw.WriteUint32(o.UI32)
	xw.WriteUint64(uint64(o.I64))
	xw.WriteUint64(o.UI64)
	xw.WriteBytes(o.BS)
	xw.WriteString(o.S)
	return xw.Tot(), xw.Error()
}

func (o *TestStruct) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *TestStruct) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *TestStruct) decodeXDR(xr *xdr.Reader) error {
	o.I = int(xr.ReadUint64())
	o.I16 = int16(xr.ReadUint16())
	o.UI16 = xr.ReadUint16()
	o.I32 = int32(xr.ReadUint32())
	o.UI32 = xr.ReadUint32()
	o.I64 = int64(xr.ReadUint64())
	o.UI64 = xr.ReadUint64()
	o.BS = xr.ReadBytes()
	o.S = xr.ReadString()
	return xr.Error()
}
