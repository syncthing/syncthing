package slogutil

import (
	"log/slog"
	"sync"
	"time"
)

var globalRecorder = &lineRecorder{}

type Recorder interface {
	Since(t time.Time) []Line
	Clear()
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
