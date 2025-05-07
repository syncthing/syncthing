package slogutil

import (
	"context"
	"log/slog"
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
		sb.WriteRune(' ')
		sb.WriteString(a.Key)
		sb.WriteRune('=')
		sb.WriteString(a.Value.Resolve().String())
	}
	var prefix string
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}
	rec.Attrs(func(a slog.Attr) bool {
		sb.WriteRune(' ')
		sb.WriteString(prefix)
		sb.WriteString(a.Key)
		sb.WriteRune('=')
		sb.WriteString(a.Value.Resolve().String())
		return true
	})
	line := Line{
		When:    rec.Time,
		Message: sb.String(),
		Level:   rec.Level,
	}
	h.rec.record(line)
	return nil
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
