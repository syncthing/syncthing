// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// A Line is our internal representation of a formatted log line. This is
// what we present in the API and what we buffer internally.
type Line struct {
	When    time.Time  `json:"when"`
	Message string     `json:"message"`
	Level   slog.Level `json:"level"`
}

func (l *Line) WriteTo(w io.Writer) (int64, error) {
	n, err := fmt.Fprintf(w, "%s %s %s\n", l.timeStr(), l.levelStr(), l.Message)
	return int64(n), err
}

func (l *Line) timeStr() string {
	return l.When.Format("2006-01-02 15:04:05")
}

func (l *Line) levelStr() string {
	str := func(base string, val slog.Level) string {
		if val == 0 {
			return base
		}
		return fmt.Sprintf("%s%+d", base, val)
	}

	switch {
	case l.Level < slog.LevelInfo:
		return str("DBG", l.Level-slog.LevelDebug)
	case l.Level < slog.LevelWarn:
		return str("INF", l.Level-slog.LevelInfo)
	case l.Level < slog.LevelError:
		return str("WRN", l.Level-slog.LevelWarn)
	default:
		return str("ERR", l.Level-slog.LevelError)
	}
}

func (l *Line) MarshalJSON() ([]byte, error) {
	// Custom marshal to get short level strings instead of default JSON serialisation
	return json.Marshal(map[string]any{
		"when":    l.When,
		"message": l.Message,
		"level":   l.levelStr(),
	})
}
