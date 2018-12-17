// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr

import "io"

// The Marshaller is a thin wrapper around a byte buffer. The buffer must be
// of sufficient size to hold the complete marshalled object, or an
// io.ErrShortBuffer error will result. The Marshal... methods don't
// individually return an error - the intention is that multiple fields are
// marshalled in rapid succession, followed by a check of the Error field on
// the Marshaller.
type Marshaller struct {
	Data  []byte
	Error error

	offset int
}

// MarshalRaw copies the raw bytes to the buffer, without a size prefix or
// padding. This is suitable for appending data already in XDR format from
// another source.
func (m *Marshaller) MarshalRaw(bs []byte) {
	if m.Error != nil {
		return
	}
	if len(m.Data) < m.offset+len(bs) {
		m.Error = io.ErrShortBuffer
		return
	}

	m.offset += copy(m.Data[m.offset:], bs)
}

// MarshalString appends the string to the buffer, with a size prefix and
// correct padding.
func (m *Marshaller) MarshalString(s string) {
	if m.Error != nil {
		return
	}
	if len(m.Data) < m.offset+4+len(s)+Padding(len(s)) {
		m.Error = io.ErrShortBuffer
		return
	}

	m.MarshalUint32(uint32(len(s)))
	m.offset += copy(m.Data[m.offset:], s)
	m.offset += copy(m.Data[m.offset:], padBytes[:Padding(len(s))])
}

// MarshalString appends the bytes to the buffer, with a size prefix and
// correct padding.
func (m *Marshaller) MarshalBytes(bs []byte) {
	if m.Error != nil {
		return
	}
	if len(m.Data) < m.offset+4+len(bs)+Padding(len(bs)) {
		m.Error = io.ErrShortBuffer
		return
	}

	m.MarshalUint32(uint32(len(bs)))
	m.offset += copy(m.Data[m.offset:], bs)
	m.offset += copy(m.Data[m.offset:], padBytes[:Padding(len(bs))])
}

// MarshalString appends the bool to the buffer, as an uint32.
func (m *Marshaller) MarshalBool(v bool) {
	if v {
		m.MarshalUint8(1)
	} else {
		m.MarshalUint8(0)
	}
}

// MarshalString appends the uint8 to the buffer, as an uint32.
func (m *Marshaller) MarshalUint8(v uint8) {
	m.MarshalUint32(uint32(v))
}

// MarshalString appends the uint16 to the buffer, as an uint32.
func (m *Marshaller) MarshalUint16(v uint16) {
	m.MarshalUint32(uint32(v))
}

// MarshalString appends the uint32 to the buffer.
func (m *Marshaller) MarshalUint32(v uint32) {
	if m.Error != nil {
		return
	}
	if len(m.Data) < m.offset+4 {
		m.Error = io.ErrShortBuffer
		return
	}

	m.Data[m.offset+0] = byte(v >> 24)
	m.Data[m.offset+1] = byte(v >> 16)
	m.Data[m.offset+2] = byte(v >> 8)
	m.Data[m.offset+3] = byte(v)
	m.offset += 4
}

// MarshalString appends the uint64 to the buffer.
func (m *Marshaller) MarshalUint64(v uint64) {
	if m.Error != nil {
		return
	}
	if len(m.Data) < m.offset+8 {
		m.Error = io.ErrShortBuffer
		return
	}

	m.Data[m.offset+0] = byte(v >> 56)
	m.Data[m.offset+1] = byte(v >> 48)
	m.Data[m.offset+2] = byte(v >> 40)
	m.Data[m.offset+3] = byte(v >> 32)
	m.Data[m.offset+4] = byte(v >> 24)
	m.Data[m.offset+5] = byte(v >> 16)
	m.Data[m.offset+6] = byte(v >> 8)
	m.Data[m.offset+7] = byte(v)
	m.offset += 8
}
