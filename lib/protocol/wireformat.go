// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"path/filepath"

	"golang.org/x/text/unicode/norm"
)

type wireFormatConnection struct {
	Connection
}

func (c wireFormatConnection) Index(folder string, fs []FileInfo) error {
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	return c.Connection.Index(folder, myFs)
}

func (c wireFormatConnection) IndexUpdate(folder string, fs []FileInfo) error {
	var myFs = make([]FileInfo, len(fs))
	copy(myFs, fs)

	for i := range fs {
		myFs[i].Name = norm.NFC.String(filepath.ToSlash(myFs[i].Name))
	}

	return c.Connection.IndexUpdate(folder, myFs)
}

func (c wireFormatConnection) Request(folder, name string, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	name = norm.NFC.String(filepath.ToSlash(name))
	return c.Connection.Request(folder, name, offset, size, hash, weakHash, fromTemporary)
}
