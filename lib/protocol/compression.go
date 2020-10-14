// Copyright (C) 2015 The Protocol Authors.

package protocol

import "fmt"

const (
	compressionThreshold = 128 // don't bother compressing messages smaller than this many bytes
)

var compressionMarshal = map[Compression]string{
	CompressionNever:    "never",
	CompressionMetadata: "metadata",
	CompressionAlways:   "always",
}

var compressionUnmarshal = map[string]Compression{
	// Legacy
	"false": CompressionNever,
	"true":  CompressionMetadata,

	// Current
	"never":    CompressionNever,
	"metadata": CompressionMetadata,
	"always":   CompressionAlways,
}

func (c Compression) GoString() string {
	return fmt.Sprintf("%q", c.String())
}

func (c Compression) MarshalText() ([]byte, error) {
	return []byte(compressionMarshal[c]), nil
}

func (c *Compression) UnmarshalText(bs []byte) error {
	*c = compressionUnmarshal[string(bs)]
	return nil
}
