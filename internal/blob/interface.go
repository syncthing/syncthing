// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package blob

import (
	"context"
	"io"
)

type Store interface {
	Upload(ctx context.Context, key string, r io.Reader) error
	Download(ctx context.Context, key string, w Writer) error
	LatestKey(ctx context.Context) (string, error)
}

type Writer interface {
	io.Writer
	io.WriterAt
}
