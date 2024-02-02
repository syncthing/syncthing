// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"context"
	"path/filepath"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/text/unicode/norm"
)

type wireFormatConnection struct {
	Connection
}

func (c wireFormatConnection) Index(ctx context.Context, idx *Index) error {
	idxCopy := proto.Clone(idx).(*Index)
	for i := range idxCopy.Files {
		idxCopy.Files[i].Name = norm.NFC.String(filepath.ToSlash(idxCopy.Files[i].Name))
	}

	return c.Connection.Index(ctx, idxCopy)
}

func (c wireFormatConnection) IndexUpdate(ctx context.Context, idxUp *IndexUpdate) error {
	idxUpCopy := proto.Clone(idxUp).(*IndexUpdate)
	for i := range idxUpCopy.Files {
		idxUpCopy.Files[i].Name = norm.NFC.String(filepath.ToSlash(idxUpCopy.Files[i].Name))
	}

	return c.Connection.IndexUpdate(ctx, idxUpCopy)
}

func (c wireFormatConnection) Request(ctx context.Context, req *Request) ([]byte, error) {
	reqCopy := proto.Clone(req).(*Request)
	reqCopy.Name = norm.NFC.String(filepath.ToSlash(reqCopy.Name))
	return c.Connection.Request(ctx, reqCopy)
}
