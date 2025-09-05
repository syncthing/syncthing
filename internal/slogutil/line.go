// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"bytes"
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

func (l *Line) WriteTo(w io.Writer, f LineFormat) (int64, error) {
	buf := new(bytes.Buffer)
	if f.LevelSyslog {
		_, _ = fmt.Fprintf(buf, "<%d>", l.syslogPriority())
	}
	if f.TimestampFormat != "" {
		buf.WriteString(l.When.Format(f.TimestampFormat))
		buf.WriteRune(' ')
	}
	if f.LevelString {
		buf.WriteString(l.levelStr())
		buf.WriteRune(' ')
	}
	buf.WriteString(l.Message)
	buf.WriteRune('\n')
	return buf.WriteTo(w)
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

func (l *Line) syslogPriority() int {
	switch {
	case l.Level < slog.LevelInfo:
		return 7
	case l.Level < slog.LevelWarn:
		return 6
	case l.Level < slog.LevelError:
		return 4
	default:
		return 3
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
