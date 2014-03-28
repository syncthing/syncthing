package protocol

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o IndexMessage) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o IndexMessage) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o IndexMessage) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.Repository) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.Repository)
	if len(o.Files) > 100000 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Files)))
	for i := range o.Files {
		o.Files[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *IndexMessage) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *IndexMessage) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *IndexMessage) decodeXDR(xr *xdr.Reader) error {
	o.Repository = xr.ReadStringMax(64)
	_FilesSize := int(xr.ReadUint32())
	if _FilesSize > 100000 {
		return xdr.ErrElementSizeExceeded
	}
	o.Files = make([]FileInfo, _FilesSize)
	for i := range o.Files {
		(&o.Files[i]).decodeXDR(xr)
	}
	return xr.Error()
}

func (o FileInfo) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o FileInfo) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o FileInfo) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.Name) > 1024 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.Name)
	xw.WriteUint32(o.Flags)
	xw.WriteUint64(uint64(o.Modified))
	xw.WriteUint64(o.Version)
	if len(o.Blocks) > 100000 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Blocks)))
	for i := range o.Blocks {
		o.Blocks[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *FileInfo) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *FileInfo) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *FileInfo) decodeXDR(xr *xdr.Reader) error {
	o.Name = xr.ReadStringMax(1024)
	o.Flags = xr.ReadUint32()
	o.Modified = int64(xr.ReadUint64())
	o.Version = xr.ReadUint64()
	_BlocksSize := int(xr.ReadUint32())
	if _BlocksSize > 100000 {
		return xdr.ErrElementSizeExceeded
	}
	o.Blocks = make([]BlockInfo, _BlocksSize)
	for i := range o.Blocks {
		(&o.Blocks[i]).decodeXDR(xr)
	}
	return xr.Error()
}

func (o BlockInfo) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o BlockInfo) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o BlockInfo) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(o.Size)
	if len(o.Hash) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteBytes(o.Hash)
	return xw.Tot(), xw.Error()
}

func (o *BlockInfo) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *BlockInfo) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *BlockInfo) decodeXDR(xr *xdr.Reader) error {
	o.Size = xr.ReadUint32()
	o.Hash = xr.ReadBytesMax(64)
	return xr.Error()
}

func (o RequestMessage) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o RequestMessage) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o RequestMessage) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.Repository) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.Repository)
	if len(o.Name) > 1024 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.Name)
	xw.WriteUint64(o.Offset)
	xw.WriteUint32(o.Size)
	return xw.Tot(), xw.Error()
}

func (o *RequestMessage) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *RequestMessage) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *RequestMessage) decodeXDR(xr *xdr.Reader) error {
	o.Repository = xr.ReadStringMax(64)
	o.Name = xr.ReadStringMax(1024)
	o.Offset = xr.ReadUint64()
	o.Size = xr.ReadUint32()
	return xr.Error()
}

func (o OptionsMessage) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o OptionsMessage) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o OptionsMessage) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.Options) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Options)))
	for i := range o.Options {
		o.Options[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *OptionsMessage) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *OptionsMessage) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *OptionsMessage) decodeXDR(xr *xdr.Reader) error {
	_OptionsSize := int(xr.ReadUint32())
	if _OptionsSize > 64 {
		return xdr.ErrElementSizeExceeded
	}
	o.Options = make([]Option, _OptionsSize)
	for i := range o.Options {
		(&o.Options[i]).decodeXDR(xr)
	}
	return xr.Error()
}

func (o Option) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o Option) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o Option) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.Key) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.Key)
	if len(o.Value) > 1024 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.Value)
	return xw.Tot(), xw.Error()
}

func (o *Option) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *Option) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *Option) decodeXDR(xr *xdr.Reader) error {
	o.Key = xr.ReadStringMax(64)
	o.Value = xr.ReadStringMax(1024)
	return xr.Error()
}
