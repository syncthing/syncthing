package scanner

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o Block) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o Block) MarshalXDR() []byte {
	return o.AppendXDR(make([]byte, 0, 128))
}

func (o Block) AppendXDR(bs []byte) []byte {
	var aw = xdr.AppendWriter(bs)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o Block) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint64(uint64(o.Offset))
	xw.WriteUint32(o.Size)
	xw.WriteBytes(o.Hash)
	return xw.Tot(), xw.Error()
}

func (o *Block) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *Block) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *Block) decodeXDR(xr *xdr.Reader) error {
	o.Offset = int64(xr.ReadUint64())
	o.Size = xr.ReadUint32()
	o.Hash = xr.ReadBytes()
	return xr.Error()
}
