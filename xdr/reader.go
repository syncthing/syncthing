package xdr

import (
	"errors"
	"io"
)

var ErrElementSizeExceeded = errors.New("element size exceeded")

type Reader struct {
	r   io.Reader
	tot int
	err error
	b   [8]byte
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: r,
	}
}

func (r *Reader) ReadString() string {
	return string(r.ReadBytes())
}

func (r *Reader) ReadStringMax(max int) string {
	return string(r.ReadBytesMax(max))
}

func (r *Reader) ReadBytes() []byte {
	return r.ReadBytesInto(nil)
}

func (r *Reader) ReadBytesMax(max int) []byte {
	return r.ReadBytesMaxInto(max, nil)
}

func (r *Reader) ReadBytesInto(dst []byte) []byte {
	return r.ReadBytesMaxInto(0, dst)
}

func (r *Reader) ReadBytesMaxInto(max int, dst []byte) []byte {
	if r.err != nil {
		return nil
	}
	l := int(r.ReadUint32())
	if r.err != nil {
		return nil
	}
	if max > 0 && l > max {
		r.err = ErrElementSizeExceeded
		return nil
	}
	if l+pad(l) > len(dst) {
		dst = make([]byte, l+pad(l))
	} else {
		dst = dst[:l+pad(l)]
	}
	_, r.err = io.ReadFull(r.r, dst)
	r.tot += l + pad(l)
	return dst[:l]
}

func (r *Reader) ReadUint16() uint16 {
	if r.err != nil {
		return 0
	}
	_, r.err = io.ReadFull(r.r, r.b[:4])
	r.tot += 4
	return uint16(r.b[1]) | uint16(r.b[0])<<8
}

func (r *Reader) ReadUint32() uint32 {
	var n int
	if r.err != nil {
		return 0
	}
	n, r.err = io.ReadFull(r.r, r.b[:4])
	if n < 4 {
		return 0
	}
	r.tot += n
	return uint32(r.b[3]) | uint32(r.b[2])<<8 | uint32(r.b[1])<<16 | uint32(r.b[0])<<24
}

func (r *Reader) ReadUint64() uint64 {
	var n int
	if r.err != nil {
		return 0
	}
	n, r.err = io.ReadFull(r.r, r.b[:8])
	r.tot += n
	return uint64(r.b[7]) | uint64(r.b[6])<<8 | uint64(r.b[5])<<16 | uint64(r.b[4])<<24 |
		uint64(r.b[3])<<32 | uint64(r.b[2])<<40 | uint64(r.b[1])<<48 | uint64(r.b[0])<<56
}

func (r *Reader) Tot() int {
	return r.tot
}

func (r *Reader) Error() error {
	return r.err
}
