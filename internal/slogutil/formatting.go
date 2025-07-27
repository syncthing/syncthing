// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
)

type formattingHandler struct {
	attrs  []slog.Attr
	groups []string
	out    io.Writer
	recs   []*lineRecorder
}

var _ slog.Handler = (*formattingHandler)(nil)

func (h *formattingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *formattingHandler) Handle(_ context.Context, rec slog.Record) error {
	var prefix string
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}

	// Collect all the attributes, with newer attributes towards the front.
	// Expand groups.
	var attrs []slog.Attr
	rec.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, expandAttrs(prefix, a)...)
		return true
	})
	for _, a := range h.attrs {
		attrs = append(attrs, expandAttrs(prefix, a)...)
	}

	// Sort and compact the attributes, so that we keep the newest of any
	// with conflicting keys.
	// slices.Reverse(attrs)
	// slices.SortStableFunc(attrs, slogKeyCompare)
	// attrs = slices.CompactFunc(attrs, func(a, b slog.Attr) bool { return a.Key == b.Key })

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
	for _, rec := range h.recs {
		rec.record(line)
	}

	// If there's an output, print the line.
	if h.out != nil {
		_, _ = line.WriteTo(h.out)
	}
	return nil
}

func expandAttrs(prefix string, a slog.Attr) []slog.Attr {
	if a.Value.Kind() != slog.KindGroup {
		return []slog.Attr{a}
	}
	prefix = prefix + a.Key + "."
	var attrs []slog.Attr
	for _, attr := range a.Value.Group() {
		attr.Key = prefix + attr.Key
		attrs = append(attrs, expandAttrs(prefix, attr)...)
	}
	return attrs
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
		recs:   h.recs,
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
		recs:   h.recs,
		out:    h.out,
	}
}
