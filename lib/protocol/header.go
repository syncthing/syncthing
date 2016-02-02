// Copyright (C) 2014 The Protocol Authors.

package protocol

import "github.com/calmh/xdr"

type header struct {
	version     int
	msgID       int
	msgType     int
	compression bool
}

func (h header) MarshalXDRInto(m *xdr.Marshaller) error {
	v := encodeHeader(h)
	m.MarshalUint32(v)
	return m.Error
}

func (h *header) UnmarshalXDRFrom(u *xdr.Unmarshaller) error {
	v := u.UnmarshalUint32()
	*h = decodeHeader(v)
	return u.Error
}

func encodeHeader(h header) uint32 {
	var isComp uint32
	if h.compression {
		isComp = 1 << 0 // the zeroth bit is the compression bit
	}
	return uint32(h.version&0xf)<<28 +
		uint32(h.msgID&0xfff)<<16 +
		uint32(h.msgType&0xff)<<8 +
		isComp
}

func decodeHeader(u uint32) header {
	return header{
		version:     int(u>>28) & 0xf,
		msgID:       int(u>>16) & 0xfff,
		msgType:     int(u>>8) & 0xff,
		compression: u&1 == 1,
	}
}
