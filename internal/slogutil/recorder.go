// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"log/slog"
	"sync"
	"time"
)

const maxLogLines = 1000

type Recorder interface {
	Since(t time.Time) []Line
	Clear()
}

func NewRecorder(level slog.Level) Recorder {
	return &lineRecorder{level: level}
}

type lineRecorder struct {
	level slog.Level
	mut   sync.Mutex
	lines []Line
}

func (r *lineRecorder) record(line Line) {
	if line.Level < r.level {
		return
	}
	r.mut.Lock()
	r.lines = append(r.lines, line)
	if len(r.lines) > maxLogLines {
		r.lines = r.lines[len(r.lines)-maxLogLines:]
	}
	r.mut.Unlock()
}

func (r *lineRecorder) Clear() {
	r.mut.Lock()
	r.lines = nil
	r.mut.Unlock()
}

func (r *lineRecorder) Since(t time.Time) []Line {
	r.mut.Lock()
	defer r.mut.Unlock()
	for i := range r.lines {
		if r.lines[i].When.After(t) {
			return r.lines[i:]
		}
	}
	return nil
}
