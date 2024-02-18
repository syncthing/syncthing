// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"context"
	"path/filepath"

	"golang.org/x/text/unicode/norm"
)

type wireFormatConnection struct {
	Connection
}

func (c wireFormatConnection) Index(ctx context.Context, folder string, fs []FileInfo) error {
	for i := range fs {
		fs[i].Name = norm.NFC.String(filepath.ToSlash(fs[i].Name))
	}

	return c.Connection.Index(ctx, folder, fs)
}

func (c wireFormatConnection) IndexUpdate(ctx context.Context, folder string, fs []FileInfo) error {
	for i := range fs {
		fs[i].Name = norm.NFC.String(filepath.ToSlash(fs[i].Name))
	}

	return c.Connection.IndexUpdate(ctx, folder, fs)
}

func (c wireFormatConnection) Request(ctx context.Context, folder string, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
	name = norm.NFC.String(filepath.ToSlash(name))
	return c.Connection.Request(ctx, folder, name, blockNo, offset, size, hash, weakHash, fromTemporary)
}
