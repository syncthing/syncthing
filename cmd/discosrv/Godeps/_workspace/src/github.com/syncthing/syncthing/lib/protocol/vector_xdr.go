// Copyright (C) 2015 The Protocol Authors.

package protocol

import "github.com/calmh/xdr"

// This stuff is hacked up manually because genxdr doesn't support 'type
// Vector []Counter' declarations and it was tricky when I tried to add it...

type xdrWriter interface {
	WriteUint32(uint32) (int, error)
	WriteUint64(uint64) (int, error)
}
type xdrReader interface {
	ReadUint32() uint32
	ReadUint64() uint64
}

// EncodeXDRInto encodes the vector as an XDR object into the given XDR
// encoder.
func (v Vector) EncodeXDRInto(w xdrWriter) (int, error) {
	w.WriteUint32(uint32(len(v)))
	for i := range v {
		w.WriteUint64(v[i].ID)
		w.WriteUint64(v[i].Value)
	}
	return 4 + 16*len(v), nil
}

// DecodeXDRFrom decodes the XDR objects from the given reader into itself.
func (v *Vector) DecodeXDRFrom(r xdrReader) error {
	l := int(r.ReadUint32())
	if l > 1e6 {
		return xdr.ElementSizeExceeded("number of counters", l, 1e6)
	}
	n := make(Vector, l)
	for i := range n {
		n[i].ID = r.ReadUint64()
		n[i].Value = r.ReadUint64()
	}
	*v = n
	return nil
}
