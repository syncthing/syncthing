package protocol

import (
	"errors"
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
	b   [8]byte
}

// We will never encode nor expect to decode blobs larger than 10 MB. Check
// inserted to protect against attempting to allocate arbitrary amounts of
// memory when reading a corrupt message.
const maxBytesFieldLength = 10 * 1 << 20

var ErrFieldLengthExceeded = errors.New("Raw bytes field size exceeds limit")

func (w *marshalWriter) writeString(s string) {
	w.writeBytes([]byte(s))
}

func (w *marshalWriter) writeBytes(bs []byte) {
	if w.err != nil {
		return
	}
	if len(bs) > maxBytesFieldLength {
		w.err = ErrFieldLengthExceeded
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
	w.b[0] = byte(v >> 24)
	w.b[1] = byte(v >> 16)
	w.b[2] = byte(v >> 8)
	w.b[3] = byte(v)
	_, w.err = w.w.Write(w.b[:4])
	w.tot += 4
}

func (w *marshalWriter) writeUint64(v uint64) {
	if w.err != nil {
		return
	}
	w.b[0] = byte(v >> 56)
	w.b[1] = byte(v >> 48)
	w.b[2] = byte(v >> 40)
	w.b[3] = byte(v >> 32)
	w.b[4] = byte(v >> 24)
	w.b[5] = byte(v >> 16)
	w.b[6] = byte(v >> 8)
	w.b[7] = byte(v)
	_, w.err = w.w.Write(w.b[:8])
	w.tot += 8
}

type marshalReader struct {
	r   io.Reader
	tot int
	err error
	b   [8]byte
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
	if l > maxBytesFieldLength {
		r.err = ErrFieldLengthExceeded
		return nil
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
	_, r.err = io.ReadFull(r.r, r.b[:4])
	r.tot += 4
	return uint32(r.b[3]) | uint32(r.b[2])<<8 | uint32(r.b[1])<<16 | uint32(r.b[0])<<24
}

func (r *marshalReader) readUint64() uint64 {
	if r.err != nil {
		return 0
	}
	_, r.err = io.ReadFull(r.r, r.b[:8])
	r.tot += 8
	return uint64(r.b[7]) | uint64(r.b[6])<<8 | uint64(r.b[5])<<16 | uint64(r.b[4])<<24 |
		uint64(r.b[3])<<32 | uint64(r.b[2])<<40 | uint64(r.b[1])<<48 | uint64(r.b[0])<<56
}
