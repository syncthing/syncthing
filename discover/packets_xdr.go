package discover

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o QueryV2) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o QueryV2) MarshalXDR() []byte {
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o QueryV2) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Magic)
	if len(o.NodeID) > 32 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteBytes(o.NodeID)
	return xw.Tot(), xw.Error()
}

func (o *QueryV2) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *QueryV2) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *QueryV2) decodeXDR(xr *xdr.Reader) error {
	o.Magic = xr.ReadUint32()
	o.NodeID = xr.ReadBytesMax(32)
	return xr.Error()
}

func (o AnnounceV2) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o AnnounceV2) MarshalXDR() []byte {
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o AnnounceV2) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Magic)
	o.This.encodeXDR(xw)
	if len(o.Extra) > 16 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Extra)))
	for i := range o.Extra {
		o.Extra[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *AnnounceV2) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *AnnounceV2) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *AnnounceV2) decodeXDR(xr *xdr.Reader) error {
	o.Magic = xr.ReadUint32()
	(&o.This).decodeXDR(xr)
	_ExtraSize := int(xr.ReadUint32())
	if _ExtraSize > 16 {
		return xdr.ErrElementSizeExceeded
	}
	o.Extra = make([]Node, _ExtraSize)
	for i := range o.Extra {
		(&o.Extra[i]).decodeXDR(xr)
	}
	return xr.Error()
}

func (o Node) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o Node) MarshalXDR() []byte {
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o Node) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.ID) > 32 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteBytes(o.ID)
	if len(o.Addresses) > 16 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Addresses)))
	for i := range o.Addresses {
		o.Addresses[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *Node) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *Node) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *Node) decodeXDR(xr *xdr.Reader) error {
	o.ID = xr.ReadBytesMax(32)
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
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
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
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *Address) decodeXDR(xr *xdr.Reader) error {
	o.IP = xr.ReadBytesMax(16)
	o.Port = xr.ReadUint16()
	return xr.Error()
}
