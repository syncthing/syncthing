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
	"strings"
)

var (
	GlobalRecorder = &lineRecorder{level: -1000}
	ErrorRecorder  = &lineRecorder{level: slog.LevelError}
	globalLevels   = &levelTracker{
		levels: make(map[string]slog.Level),
		descrs: make(map[string]string),
	}
	slogDef *slog.Logger
)

func init() {
	var out io.Writer = os.Stdout
	if os.Getenv("LOGGER_DISCARD") != "" {
		// Hack to completely disable logging, for example when running
		// benchmarks.
		out = io.Discard
	}
	slogDef = slog.New(&formattingHandler{
		recs: []*lineRecorder{GlobalRecorder, ErrorRecorder},
		out:  out,
	})
	slog.SetDefault(slogDef)

	// Handle legacy STTRACE var
	pkgs := strings.Split(os.Getenv("STTRACE"), ",")
	for _, pkg := range pkgs {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		level := slog.LevelDebug
		if cutPkg, levelStr, ok := strings.Cut(pkg, ":"); ok {
			pkg = cutPkg
			if err := level.UnmarshalText([]byte(levelStr)); err != nil {
				slog.Warn("Bad log level requested in STTRACE", slog.String("pkg", pkg), slog.String("level", levelStr), Error(err))
			}
		}
		globalLevels.Set(pkg, level)
	}
}
