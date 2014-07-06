package files

import (
	"bytes"
	"io"

	"github.com/calmh/syncthing/xdr"
)

func (o fileVersion) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o fileVersion) MarshalXDR() []byte {
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o fileVersion) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint64(o.version)
	xw.WriteBytes(o.node)
	return xw.Tot(), xw.Error()
}

func (o *fileVersion) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *fileVersion) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *fileVersion) decodeXDR(xr *xdr.Reader) error {
	o.version = xr.ReadUint64()
	o.node = xr.ReadBytes()
	return xr.Error()
}

func (o versionList) EncodeXDR(w io.Writer) (int, error) {
	var xw = xdr.NewWriter(w)
	return o.encodeXDR(xw)
}

func (o versionList) MarshalXDR() []byte {
	var aw = make(xdr.AppendWriter, 0, 128)
	var xw = xdr.NewWriter(&aw)
	o.encodeXDR(xw)
	return []byte(aw)
}

func (o versionList) encodeXDR(xw *xdr.Writer) (int, error) {
	xw.WriteUint32(uint32(len(o.versions)))
	for i := range o.versions {
		o.versions[i].encodeXDR(xw)
	}
	return xw.Tot(), xw.Error()
}

func (o *versionList) DecodeXDR(r io.Reader) error {
	xr := xdr.NewReader(r)
	return o.decodeXDR(xr)
}

func (o *versionList) UnmarshalXDR(bs []byte) error {
	var br = bytes.NewReader(bs)
	var xr = xdr.NewReader(br)
	return o.decodeXDR(xr)
}

func (o *versionList) decodeXDR(xr *xdr.Reader) error {
	_versionsSize := int(xr.ReadUint32())
	o.versions = make([]fileVersion, _versionsSize)
	for i := range o.versions {
		(&o.versions[i]).decodeXDR(xr)
	}
	return xr.Error()
}
