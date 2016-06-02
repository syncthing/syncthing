package missinggo

import "io"

type SectionWriter struct {
	w        io.WriterAt
	off, len int64
}

func NewSectionWriter(w io.WriterAt, off, len int64) *SectionWriter {
	return &SectionWriter{w, off, len}
}

func (me *SectionWriter) WriteAt(b []byte, off int64) (n int, err error) {
	if off >= me.len {
		err = io.EOF
		return
	}
	if off+int64(len(b)) > me.len {
		b = b[:me.len-off]
	}
	return me.w.WriteAt(b, me.off+off)
}
