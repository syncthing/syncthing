// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"io"
	"log/slog"
	"os"
	"time"
)

var (
	GlobalRecorder    = &lineRecorder{level: -1000}
	ErrorRecorder     = &lineRecorder{level: slog.LevelError}
	DefaultLineFormat = LineFormat{
		TimestampFormat: time.DateTime,
		LevelString:     true,
	}
	globalLevels = &levelTracker{
		levels: make(map[string]slog.Level),
		descrs: make(map[string]string),
	}
	globalFormatter = &formattingOptions{
		LineFormat: DefaultLineFormat,
		recs:       []*lineRecorder{GlobalRecorder, ErrorRecorder},
		out:        logWriter(),
	}
	slogDef = slog.New(&formattingHandler{opts: globalFormatter})
)

func logWriter() io.Writer {
	if os.Getenv("LOGGER_DISCARD") != "" {
		// Hack to completely disable logging, for example when running
		// benchmarks.
		return io.Discard
	}

	return os.Stdout
}

func init() {
	slog.SetDefault(slogDef)
}
