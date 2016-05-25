package resource

import (
	"io"
	"os"
)

// Provides access to resources through the native OS filesystem.
type OSFileProvider struct{}

var _ Provider = &OSFileProvider{}

func (me *OSFileProvider) NewInstance(filePath string) (r Instance, err error) {
	return &osFileInstance{filePath}, nil
}

type osFileInstance struct {
	path string
}

var _ Instance = &osFileInstance{}

func (me *osFileInstance) Get() (ret io.ReadCloser, err error) {
	return os.Open(me.path)
}

func (me *osFileInstance) Put(r io.Reader) (err error) {
	f, err := os.OpenFile(me.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return
}

func (me *osFileInstance) ReadAt(b []byte, off int64) (n int, err error) {
	f, err := os.Open(me.path)
	if err != nil {
		return
	}
	defer f.Close()
	return f.ReadAt(b, off)
}

func (me *osFileInstance) WriteAt(b []byte, off int64) (n int, err error) {
	f, err := os.OpenFile(me.path, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	defer f.Close()
	return f.WriteAt(b, off)
}

func (me *osFileInstance) Stat() (fi os.FileInfo, err error) {
	return os.Stat(me.path)
}

func (me *osFileInstance) Delete() error {
	return os.Remove(me.path)
}
