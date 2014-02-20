package protocol

import "github.com/calmh/syncthing/xdr"

type header struct {
	version int
	msgID   int
	msgType int
}

func (h header) encodeXDR(xw *xdr.Writer) (int, error) {
	u := encodeHeader(h)
	return xw.WriteUint32(u)
}

func (h *header) decodeXDR(xr *xdr.Reader) error {
	u := xr.ReadUint32()
	*h = decodeHeader(u)
	return xr.Error()
}

func encodeHeader(h header) uint32 {
	return uint32(h.version&0xf)<<28 +
		uint32(h.msgID&0xfff)<<16 +
		uint32(h.msgType&0xff)<<8
}

func decodeHeader(u uint32) header {
	return header{
		version: int(u>>28) & 0xf,
		msgID:   int(u>>16) & 0xfff,
		msgType: int(u>>8) & 0xff,
	}
}
