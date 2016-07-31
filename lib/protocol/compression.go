// Copyright (C) 2015 The Protocol Authors.

package protocol

import "fmt"

const (
	compressionThreshold = 128 // don't bother compressing messages smaller than this many bytes
)

var compressionMarshal = map[Compression]string{
	CompressNever:    "never",
	CompressMetadata: "metadata",
	CompressAlways:   "always",
}

var compressionUnmarshal = map[string]Compression{
	// Legacy
	"false": CompressNever,
	"true":  CompressMetadata,

	// Current
	"never":    CompressNever,
	"metadata": CompressMetadata,
	"always":   CompressAlways,
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
