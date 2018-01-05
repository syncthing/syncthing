package binutil

import (
	"io"
	"os"
)

type SizedReader interface {
	io.Reader
	Size() int64
}

type SizedFile struct {
	f *os.File
	s *io.SectionReader // helper, for Size()
}

func (r *SizedFile) Read(p []byte) (n int, err error) { return r.s.Read(p) }
func (r *SizedFile) Size() int64                      { return r.s.Size() }
func (r *SizedFile) Close() error                     { return r.f.Close() }

func SizedOpen(filename string) (*SizedFile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return &SizedFile{
		f: f,
		s: io.NewSectionReader(f, 0, info.Size()),
	}, nil
}
