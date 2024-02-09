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

func (c wireFormatConnection) Index(ctx context.Context, idx *Index) error {
	// XXX: this copying needs to happen because of optimisations in
	// FileInfoBatch. We should invert that, make the FileInfoBatch simpler,
	// and skip this copying.
	filesCopy := make([]FileInfo, len(idx.Files))
	copy(filesCopy, idx.Files)
	idx.Files = filesCopy
	for i := range idx.Files {
		idx.Files[i].Name = norm.NFC.String(filepath.ToSlash(idx.Files[i].Name))
	}

	return c.Connection.Index(ctx, idx)
}

func (c wireFormatConnection) IndexUpdate(ctx context.Context, idxUp *IndexUpdate) error {
	// XXX: same comment as above
	filesCopy := make([]FileInfo, len(idxUp.Files))
	copy(filesCopy, idxUp.Files)
	idxUp.Files = filesCopy
	for i := range idxUp.Files {
		idxUp.Files[i].Name = norm.NFC.String(filepath.ToSlash(idxUp.Files[i].Name))
	}

	return c.Connection.IndexUpdate(ctx, idxUp)
}

func (c wireFormatConnection) Request(ctx context.Context, req *Request) ([]byte, error) {
	req.Name = norm.NFC.String(filepath.ToSlash(req.Name))
	return c.Connection.Request(ctx, req)
}
