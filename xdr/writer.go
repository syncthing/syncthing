package xdr

import "io"

func pad(l int) int {
	d := l % 4
	if d == 0 {
		return 0
	}
	return 4 - d
}

var padBytes = []byte{0, 0, 0}

type Writer struct {
	w   io.Writer
	tot int
	err error
	b   [8]byte
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

func (w *Writer) WriteString(s string) (int, error) {
	return w.WriteBytes([]byte(s))
}

func (w *Writer) WriteBytes(bs []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	w.WriteUint32(uint32(len(bs)))
	if w.err != nil {
		return 0, w.err
	}

	var l, n int
	n, w.err = w.w.Write(bs)
	l += n

	if p := pad(len(bs)); w.err == nil && p > 0 {
		n, w.err = w.w.Write(padBytes[:p])
		l += n
	}

	w.tot += l
	return l, w.err
}

func (w *Writer) WriteUint16(v uint16) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.b[0] = byte(v >> 8)
	w.b[1] = byte(v)
	w.b[2] = 0
	w.b[3] = 0

	var l int
	l, w.err = w.w.Write(w.b[:4])
	w.tot += l
	return l, w.err
}

func (w *Writer) WriteUint32(v uint32) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.b[0] = byte(v >> 24)
	w.b[1] = byte(v >> 16)
	w.b[2] = byte(v >> 8)
	w.b[3] = byte(v)

	var l int
	l, w.err = w.w.Write(w.b[:4])
	w.tot += l
	return l, w.err
}

func (w *Writer) WriteUint64(v uint64) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.b[0] = byte(v >> 56)
	w.b[1] = byte(v >> 48)
	w.b[2] = byte(v >> 40)
	w.b[3] = byte(v >> 32)
	w.b[4] = byte(v >> 24)
	w.b[5] = byte(v >> 16)
	w.b[6] = byte(v >> 8)
	w.b[7] = byte(v)

	var l int
	l, w.err = w.w.Write(w.b[:8])
	w.tot += l
	return l, w.err
}

func (w *Writer) Tot() int {
	return w.tot
}

func (w *Writer) Error() error {
	return w.err
}
