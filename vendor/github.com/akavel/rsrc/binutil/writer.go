package binutil

import (
	"encoding/binary"
	"io"
	"reflect"
)

type Writer struct {
	W      io.Writer
	Offset uint32 //FIXME: int64?
	Err    error
}

func (w *Writer) WriteLE(v interface{}) {
	if w.Err != nil {
		return
	}
	w.Err = binary.Write(w.W, binary.LittleEndian, v)
	if w.Err != nil {
		return
	}
	w.Offset += uint32(reflect.TypeOf(v).Size())
}

func (w *Writer) WriteFromSized(r SizedReader) {
	if w.Err != nil {
		return
	}
	var n int64
	n, w.Err = io.CopyN(w.W, r, r.Size())
	w.Offset += uint32(n)
}
