package slogutil

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var globalRecorder = &lineRecorder{}

type Recorder interface {
	Since(t time.Time) []Line
	Clear()
}

type formattingHandler struct {
	attrs  []slog.Attr
	groups []string
	out    io.Writer
	rec    *lineRecorder
}

var s slog.Handler = (*formattingHandler)(nil)

func (h *formattingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *formattingHandler) Handle(_ context.Context, rec slog.Record) error {
	var prefix string
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}

	// Collect all the attributes, with newer attributes towards the end.
	var attrs []slog.Attr
	rec.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	attrs = append(attrs, h.attrs...)

	// Sort and compact the attributes, so that we keep the newest of any
	// with conflicting keys.
	slices.Reverse(attrs)
	slices.SortStableFunc(attrs, func(a, b slog.Attr) int { return strings.Compare(a.Key, b.Key) })
	attrs = slices.CompactFunc(attrs, func(a, b slog.Attr) bool { return a.Key == b.Key })

	// Build the message string.
	var sb strings.Builder
	sb.WriteString(rec.Message)
	for _, attr := range attrs {
		appendAttr(&sb, prefix, attr)
	}

	line := Line{
		When:    rec.Time,
		Message: sb.String(),
		Level:   rec.Level,
	}

	// If there is a recorder, record the line.
	if h.rec != nil {
		h.rec.record(line)
	}

	// If there's an output, print the line.
	if h.out != nil {
		line.WriteTo(h.out)
	}
	return nil
}

func appendAttr(sb *strings.Builder, prefix string, a slog.Attr) {
	if a.Value.Kind() == slog.KindGroup {
		prefix := prefix + a.Key + "."
		for _, attr := range a.Value.Group() {
			appendAttr(sb, prefix, attr)
		}
		return
	}

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

func (h *formattingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.groups) > 0 {
		prefix := strings.Join(h.groups, ".") + "."
		for i := range attrs {
			attrs[i].Key = prefix + attrs[i].Key
		}
	}
	return &formattingHandler{
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
		rec:    h.rec,
		out:    h.out,
	}
}

func (h *formattingHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &formattingHandler{
		attrs:  h.attrs,
		groups: append(h.groups, name),
		rec:    h.rec,
		out:    h.out,
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
