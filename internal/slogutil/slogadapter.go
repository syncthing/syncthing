// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

var slogDef = slog.New(&FormattingHandler{
	recs: []*lineRecorder{GlobalRecorder, ErrorRecorder},
	out:  os.Stdout,
})

func init() {
	slog.SetDefault(slogDef)
}

// Log levels:
// - DEBUG: programmers only (not user troubleshooting)
// - INFO: most stuff, files syncing properly
// - WARN: errors that can be ignored or will be retried (e.g., sync failures)
// - ERROR: errors that need handling, shown in the GUI

func NewAdapter(descr string) *adapter {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	pc := pcs[0]
	fr := runtime.CallersFrames([]uintptr{pc})
	if fram, _ := fr.Next(); fram.Function != "" {
		pkgName, _ := funcNameToPkg(fram.Function)
		globalLevels.SetDescr(pkgName, descr)
	}
	return &adapter{slogDef}
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

type adapter struct {
	*slog.Logger
}

func (a adapter) Debugln(vals ...interface{}) {
	a.log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelDebug)
}

func (a adapter) Debugf(format string, vals ...interface{}) {
	a.log(fmt.Sprintf(format, vals...), slog.LevelDebug)
}

func (a adapter) Verboseln(vals ...interface{}) {
	a.log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelInfo)
}

func (a adapter) Verbosef(format string, vals ...interface{}) {
	a.log(fmt.Sprintf(format, vals...), slog.LevelInfo)
}

func (a adapter) Infoln(vals ...interface{}) {
	a.log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelInfo)
}

func (a adapter) Infof(format string, vals ...interface{}) {
	a.log(fmt.Sprintf(format, vals...), slog.LevelInfo)
}

func (a adapter) Warnln(vals ...interface{}) {
	a.log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelError)
}

func (a adapter) Warnf(format string, vals ...interface{}) {
	a.log(fmt.Sprintf(format, vals...), slog.LevelError)
}

func (a adapter) log(msg string, level slog.Level) {
	h := a.Handler()
	if !h.Enabled(context.Background(), level) {
		return
	}
	var pcs [1]uintptr
	// skip [runtime.Callers, this function, this function's caller]
	runtime.Callers(3, pcs[:])
	pc := pcs[0]
	r := slog.NewRecord(time.Now(), level, msg, pc)
	_ = h.Handle(context.Background(), r)
}

func (a adapter) ShouldDebug(facility string) bool {
	return globalLevels.Get(facility) >= slog.LevelDebug
}
