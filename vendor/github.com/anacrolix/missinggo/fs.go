package missinggo

import (
	"io"
	"os"
)

type FileStore interface {
	OpenFile(path string, flags int) (File, error)
	Stat(path string) (os.FileInfo, error)
	Rename(from, to string) error
	Remove(path string) error
}

type File interface {
	io.ReaderAt
	io.WriterAt
	io.Writer
	io.Reader
	io.Closer
	io.Seeker
	Stat() (os.FileInfo, error)
}
