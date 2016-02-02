// Copyright (C) 2015 The Protocol Authors.

package protocol

import "github.com/calmh/xdr"

// This stuff is hacked up manually because genxdr doesn't support 'type
// Vector []Counter' declarations and it was tricky when I tried to add it...

func (v Vector) MarshalXDRInto(m *xdr.Marshaller) error {
	m.MarshalUint32(uint32(len(v)))
	for i := range v {
		m.MarshalUint64(uint64(v[i].ID))
		m.MarshalUint64(v[i].Value)
	}
	return m.Error
}

func (v *Vector) UnmarshalXDRFrom(u *xdr.Unmarshaller) error {
	l := int(u.UnmarshalUint32())
	if l > 1e6 {
		return xdr.ElementSizeExceeded("number of counters", l, 1e6)
	}
	n := make(Vector, l)
	for i := range n {
		n[i].ID = ShortID(u.UnmarshalUint64())
		n[i].Value = u.UnmarshalUint64()
	}
	*v = n
	return u.Error
}

func (v Vector) XDRSize() int {
	return 4 + 16*len(v)
}
