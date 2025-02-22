// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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
	for i := range idx.Files {
		idx.Files[i].Name = norm.NFC.String(filepath.ToSlash(idx.Files[i].Name))
	}

	return c.Connection.Index(ctx, idx)
}

func (c wireFormatConnection) IndexUpdate(ctx context.Context, idxUp *IndexUpdate) error {
	for i := range idxUp.Files {
		idxUp.Files[i].Name = norm.NFC.String(filepath.ToSlash(idxUp.Files[i].Name))
	}

	return c.Connection.IndexUpdate(ctx, idxUp)
}

func (c wireFormatConnection) Request(ctx context.Context, req *Request) ([]byte, error) {
	req.Name = norm.NFC.String(filepath.ToSlash(req.Name))
	return c.Connection.Request(ctx, req)
}
