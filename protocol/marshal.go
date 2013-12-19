package protocol

import (
	"io"

	"github.com/calmh/syncthing/buffers"
)

func pad(l int) int {
	d := l % 4
	if d == 0 {
		return 0
	}
	return 4 - d
}

var padBytes = []byte{0, 0, 0}

type marshalWriter struct {
	w   io.Writer
	tot int
	err error
}

func (w *marshalWriter) writeString(s string) {
	w.writeBytes([]byte(s))
}

func (w *marshalWriter) writeBytes(bs []byte) {
	if w.err != nil {
		return
	}
	w.writeUint32(uint32(len(bs)))
	if w.err != nil {
		return
	}
	_, w.err = w.w.Write(bs)
	if p := pad(len(bs)); w.err == nil && p > 0 {
		_, w.err = w.w.Write(padBytes[:p])
	}
	w.tot += len(bs) + pad(len(bs))
}

func (w *marshalWriter) writeUint32(v uint32) {
	if w.err != nil {
		return
	}
	var b [4]byte
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
	_, w.err = w.w.Write(b[:])
	w.tot += 4
}

func (w *marshalWriter) writeUint64(v uint64) {
	if w.err != nil {
		return
	}
	var b [8]byte
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
	_, w.err = w.w.Write(b[:])
	w.tot += 8
}

type marshalReader struct {
	r   io.Reader
	tot int
	err error
}

func (r *marshalReader) readString() string {
	bs := r.readBytes()
	defer buffers.Put(bs)
	return string(bs)
}

func (r *marshalReader) readBytes() []byte {
	if r.err != nil {
		return nil
	}
	l := int(r.readUint32())
	if r.err != nil {
		return nil
	}
	if l > 10*1<<20 {
		// Individual blobs in BEP are not significantly larger than BlockSize.
		// BlockSize is not larger than 1MB.
		panic("too large read - protocol error or out of sync")
	}
	b := buffers.Get(l + pad(l))
	_, r.err = io.ReadFull(r.r, b)
	r.tot += int(l + pad(l))
	return b[:l]
}

func (r *marshalReader) readUint32() uint32 {
	if r.err != nil {
		return 0
	}
	var b [4]byte
	_, r.err = io.ReadFull(r.r, b[:])
	r.tot += 4
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}

func (r *marshalReader) readUint64() uint64 {
	if r.err != nil {
		return 0
	}
	var b [8]byte
	_, r.err = io.ReadFull(r.r, b[:])
	r.tot += 8
	return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
		uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
}
