// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"cmp"
	"context"
	"io"
	"log/slog"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type formattingHandler struct {
	attrs        []slog.Attr
	groups       []string
	out          io.Writer
	recs         []*lineRecorder
	timeOverride time.Time
}

var _ slog.Handler = (*formattingHandler)(nil)

func (h *formattingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *formattingHandler) Handle(_ context.Context, rec slog.Record) error {
	fr := runtime.CallersFrames([]uintptr{rec.PC})
	var srcAttrs []slog.Attr
	if fram, _ := fr.Next(); fram.Function != "" {
		pkgName, typeName := funcNameToPkg(fram.Function)
		lvl := globalLevels.Get(pkgName)
		if lvl > rec.Level {
			// Logging not enabled at the record's level
			return nil
		}
		srcAttrs = append(srcAttrs, slog.String("pkg", pkgName))
		if lvl <= slog.LevelDebug {
			// We are debugging, add additional source line data
			if typeName != "" {
				srcAttrs = append(srcAttrs, slog.String("type", typeName))
			}
			srcAttrs = append(srcAttrs, slog.String("file", path.Base(fram.File)), slog.Int("line", fram.Line))
		}
	}

	var prefix string
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}

	// Build the message string.
	var sb strings.Builder
	sb.WriteString(rec.Message)

	var attrCount int

	// Collect all the attributes. Expand groups. Record attributes are
	// qualified with the handler groups.
	rec.Attrs(func(a slog.Attr) bool {
		for _, attr := range expandAttrs("", a) {
			appendAttr(&sb, prefix, attr, &attrCount)
		}
		return true
	})

	// Add already existing handler attributes; no prefix, because they are
	// already prefixed.
	for _, a := range h.attrs {
		for _, attr := range expandAttrs("", a) {
			appendAttr(&sb, "", attr, &attrCount)
		}
	}

	// Add attributes for the logging package and type name
	for _, attr := range srcAttrs {
		appendAttr(&sb, "src.", attr, &attrCount)
	}

	if attrCount > 0 {
		sb.WriteRune(')')
	}

	line := Line{
		When:    cmp.Or(h.timeOverride, rec.Time),
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
	if prefix != "" {
		a.Key = prefix + "." + a.Key
	}
	val := a.Value.Resolve()
	if val.Kind() != slog.KindGroup {
		return []slog.Attr{a}
	}
	var attrs []slog.Attr
	for _, attr := range val.Group() {
		attrs = append(attrs, expandAttrs(a.Key, attr)...)
	}
	return attrs
}

func appendAttr(sb *strings.Builder, prefix string, a slog.Attr, attrCount *int) {
	if a.Key == "" {
		return
	}
	sb.WriteRune(' ')
	if *attrCount == 0 {
		sb.WriteRune('(')
	}
	sb.WriteString(prefix)
	sb.WriteString(a.Key)
	sb.WriteRune('=')
	v := a.Value.Resolve().String()
	if strings.ContainsAny(v, ` "`) {
		v = strconv.Quote(v)
	}
	sb.WriteString(v)
	*attrCount++
}

func (h *formattingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.groups) > 0 {
		prefix := strings.Join(h.groups, ".") + "."
		for i := range attrs {
			attrs[i].Key = prefix + attrs[i].Key
		}
	}
	return &formattingHandler{
		attrs:        append(h.attrs, attrs...),
		groups:       h.groups,
		recs:         h.recs,
		out:          h.out,
		timeOverride: h.timeOverride,
	}
}

func (h *formattingHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &formattingHandler{
		attrs:        h.attrs,
		groups:       append([]string{name}, h.groups...),
		recs:         h.recs,
		out:          h.out,
		timeOverride: h.timeOverride,
	}
}

func funcNameToPkg(fn string) (string, string) {
	fn = strings.ToLower(fn)
	fn = strings.TrimPrefix(fn, "github.com/syncthing/syncthing/lib/")
	fn = strings.TrimPrefix(fn, "github.com/syncthing/syncthing/internal/")

	pkgTypFn := strings.Split(fn, ".") // [package, type, method] or [package, function]
	if len(pkgTypFn) <= 2 {
		return pkgTypFn[0], ""
	}

	pkg := pkgTypFn[0]
	// Remove parenthesis and asterisk from the type name
	typ := strings.TrimLeft(strings.TrimRight(pkgTypFn[1], ")"), "(*")
	// Skip certain type names that add no value
	typ = strings.TrimSuffix(typ, "service")
	switch typ {
	case pkg, "", "serveparams":
		return pkg, ""
	default:
		return pkg, typ
	}
}
