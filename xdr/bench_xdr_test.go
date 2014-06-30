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
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o XDRBenchStruct) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint64(o.I1)
	xw.WriteUint32(o.I2)
	xw.WriteUint16(o.I3)
	if len(o.Bs0) > 128 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteBytes(o.Bs0)
	xw.WriteBytes(o.Bs1)
	if len(o.S0) > 128 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.S0)
	xw.WriteString(o.S1)
	return xw.Tot(), xw.Error()
}

func (o *XDRBenchStruct) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *XDRBenchStruct) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *XDRBenchStruct) decodeXDR(xr *xdr.Reader) error {
	o.I1 = xr.ReadUint64()
	o.I2 = xr.ReadUint32()
	o.I3 = xr.ReadUint16()
	o.Bs0 = xr.ReadBytesMax(128)
	o.Bs1 = xr.ReadBytes()
	o.S0 = xr.ReadStringMax(128)
	o.S1 = xr.ReadString()
	return xr.Error()
}

func (o repeatReader) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o repeatReader) MarshalXDR() []byte {
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o repeatReader) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteBytes(o.data)
	return xw.Tot(), xw.Error()
}

func (o *repeatReader) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *repeatReader) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *repeatReader) decodeXDR(xr *xdr.Reader) error {
	o.data = xr.ReadBytes()
	return xr.Error()
}
