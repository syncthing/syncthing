// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package protocol

import "github.com/calmh/xdr"

type header struct {
	version     int
	msgID       int
	msgType     int
	compression bool
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
