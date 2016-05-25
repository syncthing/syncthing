package missinggo

import "io"

type StatWriter struct {
	Written int64
	w       io.Writer
}

func (me *StatWriter) Write(b []byte) (n int, err error) {
	n, err = me.w.Write(b)
	me.Written += int64(n)
	return
}

func NewStatWriter(w io.Writer) *StatWriter {
	return &StatWriter{w: w}
}

var ZeroReader zeroReader

type zeroReader struct{}

func (me zeroReader) Read(b []byte) (n int, err error) {
	for i := range b {
		b[i] = 0
	}
	n = len(b)
	return
}
