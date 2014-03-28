package protocol

import (
	"path/filepath"

	"code.google.com/p/go.text/unicode/norm"
)

type wireFormatConnection struct {
	next Connection
}

func (c wireFormatConnection) ID() string {
	return c.next.ID()
}

func (c wireFormatConnection) Index(node string, fs []FileInfo) {
	for i := range fs {
		fs[i].Name = norm.NFC.String(filepath.ToSlash(fs[i].Name))
	}
	c.next.Index(node, fs)
}

func (c wireFormatConnection) Request(repo, name string, offset int64, size int) ([]byte, error) {
	name = norm.NFC.String(filepath.ToSlash(name))
	return c.next.Request(repo, name, offset, size)
}

func (c wireFormatConnection) Statistics() Statistics {
	return c.next.Statistics()
}

func (c wireFormatConnection) Option(key string) string {
	return c.next.Option(key)
}
