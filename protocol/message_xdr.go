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
	if len(o.Files) > 1000000 {
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
	if _FilesSize > 1000000 {
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

func (o ClusterConfigMessage) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o ClusterConfigMessage) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o ClusterConfigMessage) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.ClientName) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.ClientName)
	if len(o.ClientVersion) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.ClientVersion)
	if len(o.Repositories) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Repositories)))
	for i := range o.Repositories {
		o.Repositories[i].encodeXDR(xw)
	}
	if len(o.Options) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Options)))
	for i := range o.Options {
		o.Options[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *ClusterConfigMessage) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *ClusterConfigMessage) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *ClusterConfigMessage) decodeXDR(xr *xdr.Reader) error {
	o.ClientName = xr.ReadStringMax(64)
	o.ClientVersion = xr.ReadStringMax(64)
	_RepositoriesSize := int(xr.ReadUint32())
	if _RepositoriesSize > 64 {
		return xdr.ErrElementSizeExceeded
	}
	o.Repositories = make([]Repository, _RepositoriesSize)
	for i := range o.Repositories {
		(&o.Repositories[i]).decodeXDR(xr)
	}
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

func (o Repository) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o Repository) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o Repository) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.ID) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.ID)
	if len(o.Nodes) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteUint32(uint32(len(o.Nodes)))
	for i := range o.Nodes {
		o.Nodes[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *Repository) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *Repository) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *Repository) decodeXDR(xr *xdr.Reader) error {
	o.ID = xr.ReadStringMax(64)
	_NodesSize := int(xr.ReadUint32())
	if _NodesSize > 64 {
		return xdr.ErrElementSizeExceeded
	}
	o.Nodes = make([]Node, _NodesSize)
	for i := range o.Nodes {
		(&o.Nodes[i]).decodeXDR(xr)
	}
	return xr.Error()
}

func (o Node) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o Node) MarshalXDR() []byte {
	var buf bytes.Buffer
	var xw = xdr.NewWriter(&buf)
	o.encodeXDR(xw)
	return buf.Bytes()
}

func (o Node) encodeXDR(xw *xdr.Writer) (int, error) {
	if len(o.ID) > 64 {
		return xw.Tot(), xdr.ErrElementSizeExceeded
	}
	xw.WriteString(o.ID)
	xw.WriteUint32(o.Flags)
	return xw.Tot(), xw.Error()
}

func (o *Node) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *Node) UnmarshalXDR(bs []byte) error {
	var buf = bytes.NewBuffer(bs)
	var xr = xdr.NewReader(buf)
	return o.decodeXDR(xr)
}

func (o *Node) decodeXDR(xr *xdr.Reader) error {
	o.ID = xr.ReadStringMax(64)
	o.Flags = xr.ReadUint32()
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
