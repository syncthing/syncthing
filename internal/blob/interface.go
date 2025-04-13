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
