package discover

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o QueryV1) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o QueryV1) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o QueryV1) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Magic)
	if len(o.NodeID) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.NodeID)
	return xw.Tot(), xw.Error()
}

func (o *QueryV1) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *QueryV1) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *QueryV1) decodeXDR(xr *xdr.Reader) error {
	o.Magic = xr.ReadUint32()
	o.NodeID = xr.ReadStringMax(64)
	return xr.Error()
}

func (o AnnounceV1) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o AnnounceV1) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o AnnounceV1) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Magic)
	xw.WriteUint16(o.Port)
	if len(o.NodeID) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.NodeID)
	if len(o.IP) > 16 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteBytes(o.IP)
	return xw.Tot(), xw.Error()
}

func (o *AnnounceV1) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *AnnounceV1) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *AnnounceV1) decodeXDR(xr *xdr.Reader) error {
	o.Magic = xr.ReadUint32()
	o.Port = xr.ReadUint16()
	o.NodeID = xr.ReadStringMax(64)
	o.IP = xr.ReadBytesMax(16)
	return xr.Error()
}

func (o QueryV2) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o QueryV2) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o QueryV2) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Magic)
	if len(o.NodeID) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.NodeID)
	return xw.Tot(), xw.Error()
}

func (o *QueryV2) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *QueryV2) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *QueryV2) decodeXDR(xr *xdr.Reader) error {
	o.Magic = xr.ReadUint32()
	o.NodeID = xr.ReadStringMax(64)
	return xr.Error()
}

func (o AnnounceV2) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o AnnounceV2) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o AnnounceV2) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Magic)
	if len(o.NodeID) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.NodeID)
	if len(o.Addresses) > 16 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Addresses)))
	for i := range o.Addresses {
		o.Addresses[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *AnnounceV2) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *AnnounceV2) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *AnnounceV2) decodeXDR(xr *xdr.Reader) error {
	o.Magic = xr.ReadUint32()
	o.NodeID = xr.ReadStringMax(64)
	_AddressesSize := int(xr.ReadUint32())
	if _AddressesSize > 16 {
		return xdr.ErrElementSizeExceeded
	}
	o.Addresses = make([]Address, _AddressesSize)
	for i := range o.Addresses {
		(&o.Addresses[i]).decodeXDR(xr)
	}
	return xr.Error()
}

func (o Address) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o Address) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o Address) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.IP) > 16 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteBytes(o.IP)
	xw.WriteUint16(o.Port)
	return xw.Tot(), xw.Error()
}

func (o *Address) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *Address) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *Address) decodeXDR(xr *xdr.Reader) error {
	o.IP = xr.ReadBytesMax(16)
	o.Port = xr.ReadUint16()
	return xr.Error()
}
