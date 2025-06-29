package slogutil

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var globalRecorder = &lineRecorder{}

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

type Recorder interface {
	Since(t time.Time) []Line
	Clear()
}

type recordingHandler struct {
	attrs  []slog.Attr
	groups []string
	rec    *lineRecorder
}

var s slog.Handler = (*recordingHandler)(nil)

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *recordingHandler) Handle(_ context.Context, rec slog.Record) error {
	var sb strings.Builder
	sb.WriteString(rec.Message)
	for _, a := range h.attrs {
		appendAttr(&sb, "", a)
	}
	var prefix string
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}
	rec.Attrs(func(a slog.Attr) bool {
		appendAttr(&sb, prefix, a)
		return true
	})
	line := Line{
		When:    rec.Time,
		Message: sb.String(),
		Level:   rec.Level,
	}
	if h.rec != nil {
		h.rec.record(line)
	}
	line.WriteTo(os.Stdout)
	return nil
}

func appendAttr(sb *strings.Builder, prefix string, a slog.Attr) {
	sb.WriteRune(' ')
	sb.WriteString(prefix)
	sb.WriteString(a.Key)
	sb.WriteRune('=')
	v := a.Value.Resolve().String()
	if strings.ContainsRune(v, ' ') {
		v = strconv.Quote(v)
	}
	sb.WriteString(v)
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.groups) > 0 {
		prefix := strings.Join(h.groups, ".") + "."
		for i := range attrs {
			attrs[i].Key = prefix + attrs[i].Key
		}
	}
	return &recordingHandler{
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
		rec:    h.rec,
	}
}

func (h *recordingHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &recordingHandler{
		attrs:  h.attrs,
		groups: append(h.groups, name),
		rec:    h.rec,
	}
}

type lineRecorder struct {
	mut    sync.Mutex
	lines  []Line
	errors []Line
}

func (r *lineRecorder) record(line Line) {
	r.mut.Lock()
	r.lines = append(r.lines, line)
	if line.Level >= slog.LevelError {
		r.errors = append(r.errors, line)
	}
	r.mut.Unlock()
}
