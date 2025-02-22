// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"github.com/syncthing/syncthing/lib/protocol"
)

type Compression int32

const (
	CompressionMetadata Compression = 0
	CompressionNever    Compression = 1
	CompressionAlways   Compression = 2
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

func (c Compression) MarshalText() ([]byte, error) {
	return []byte(compressionMarshal[c]), nil
}

func (c *Compression) UnmarshalText(bs []byte) error {
	*c = compressionUnmarshal[string(bs)]
	return nil
}

func (c Compression) ToProtocol() protocol.Compression {
	switch c {
	case CompressionNever:
		return protocol.CompressionNever
	case CompressionAlways:
		return protocol.CompressionAlways
	case CompressionMetadata:
		return protocol.CompressionMetadata
	default:
		return protocol.CompressionMetadata
	}
}
