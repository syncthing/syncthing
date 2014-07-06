package scanner

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o File) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o File) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o File) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteString(o.Name)
	xw.WriteUint32(o.Flags)
	xw.WriteUint64(uint64(o.Modified))
	xw.WriteUint64(o.Version)
	xw.WriteUint64(uint64(o.Size))
	xw.WriteUint32(uint32(len(o.Blocks)))
	for i := range o.Blocks {
		o.Blocks[i].encodeXDR(xw)
	}
	xw.WriteBool(o.Suppressed)
	return xw.Tot(), xw.Error()
}

func (o *File) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *File) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *File) decodeXDR(xr *xdr.Reader) error {
	o.Name = xr.ReadString()
	o.Flags = xr.ReadUint32()
	o.Modified = int64(xr.ReadUint64())
	o.Version = xr.ReadUint64()
	o.Size = int64(xr.ReadUint64())
	_BlocksSize := int(xr.ReadUint32())
	o.Blocks = make([]Block, _BlocksSize)
	for i := range o.Blocks {
		(&o.Blocks[i]).decodeXDR(xr)
	}
	o.Suppressed = xr.ReadBool()
	return xr.Error()
}
