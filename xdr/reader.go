package xdr

import "io"

type Reader struct {
	r   io.Reader
	tot uint64
	err error
	b   [8]byte
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: r,
	}
}

func (r *Reader) ReadString() string {
	return string(r.ReadBytes(nil))
}

func (r *Reader) ReadBytes(dst []byte) []byte {
	if r.err != nil {
		return nil
	}
	l := int(r.ReadUint32())
	if r.err != nil {
		return nil
	}
	if l+pad(l) > len(dst) {
		dst = make([]byte, l+pad(l))
	} else {
		dst = dst[:l+pad(l)]
	}
	_, r.err = io.ReadFull(r.r, dst)
	r.tot += uint64(l + pad(l))
	return dst[:l]
}

func (r *Reader) ReadUint32() uint32 {
	if r.err != nil {
		return 0
	}
	_, r.err = io.ReadFull(r.r, r.b[:4])
	r.tot += 8
	return uint32(r.b[3]) | uint32(r.b[2])<<8 | uint32(r.b[1])<<16 | uint32(r.b[0])<<24
}

func (r *Reader) ReadUint64() uint64 {
	if r.err != nil {
		return 0
	}
	_, r.err = io.ReadFull(r.r, r.b[:8])
	r.tot += 8
	return uint64(r.b[7]) | uint64(r.b[6])<<8 | uint64(r.b[5])<<16 | uint64(r.b[4])<<24 |
		uint64(r.b[3])<<32 | uint64(r.b[2])<<40 | uint64(r.b[1])<<48 | uint64(r.b[0])<<56
}

func (r *Reader) Tot() uint64 {
	return r.tot
}

func (r *Reader) Err() error {
	return r.err
}
