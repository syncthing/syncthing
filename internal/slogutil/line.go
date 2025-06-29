package slogutil

import (
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
