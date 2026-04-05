// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"log/slog"
	"maps"
	"slices"
)

func Address(v any) slog.Attr {
	return slog.Any("address", v)
}

func Error(err any) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.Any("error", err)
}

func FilePath(path string) slog.Attr {
	return slog.String("path", path)
}

func URI(v any) slog.Attr {
	return slog.Any("uri", v)
}

func Map[T any](m map[string]T) []any {
	var attrs []any
	for _, key := range slices.Sorted(maps.Keys(m)) {
		attrs = append(attrs, slog.Any(key, m[key]))
	}
	return attrs
}
