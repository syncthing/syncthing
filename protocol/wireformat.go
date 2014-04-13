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
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	c.next.Index(node, myFs)
}

func (c wireFormatConnection) Request(repo, name string, offset int64, size int) ([]byte, error) {
	name = norm.NFC.String(filepath.ToSlash(name))
	return c.next.Request(repo, name, offset, size)
}

func (c wireFormatConnection) ClusterConfig(config ClusterConfigMessage) {
	c.next.ClusterConfig(config)
}

func (c wireFormatConnection) Statistics() Statistics {
	return c.next.Statistics()
}
