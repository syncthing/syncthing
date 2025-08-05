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
	"runtime"
	"strings"
	"time"
)

// Log levels:
// - DEBUG: programmers only (not user troubleshooting)
// - INFO: most stuff, files syncing properly
// - WARN: errors that can be ignored or will be retried (e.g., sync failures)
// - ERROR: errors that need handling, shown in the GUI

func RegisterPackage(descr string) {
	registerPackage(descr, 2)
}

func NewAdapter(descr string) *adapter {
	registerPackage(descr, 2)
	return &adapter{slogDef}
}

func registerPackage(descr string, frames int) {
	var pcs [1]uintptr
	runtime.Callers(1+frames, pcs[:])
	pc := pcs[0]
	fr := runtime.CallersFrames([]uintptr{pc})
	if fram, _ := fr.Next(); fram.Function != "" {
		pkgName, _ := funcNameToPkg(fram.Function)
		globalLevels.SetDescr(pkgName, descr)
	}
}

type adapter struct {
	l *slog.Logger
}

func (a adapter) Debugln(vals ...interface{}) {
	a.log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelDebug)
}

func (a adapter) Debugf(format string, vals ...interface{}) {
	a.log(fmt.Sprintf(format, vals...), slog.LevelDebug)
}

func (a adapter) log(msg string, level slog.Level) {
	h := a.l.Handler()
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
	return globalLevels.Get(facility) <= slog.LevelDebug
}
