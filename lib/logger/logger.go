// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

//go:generate -command counterfeiter go run github.com/maxbrunsfeld/counterfeiter/v6
//go:generate counterfeiter -o mocks/logger.go --fake-name Recorder . Recorder

// Package logger implements a standardized logger with callback functionality
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// This package uses stdlib sync as it may be used to debug syncthing/lib/sync
// and that would cause an implosion of the universe.

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelVerbose
	LevelInfo
	LevelWarn
	NumLevels
)

const (
	DefaultFlags = log.Ltime | log.Ldate
	DebugFlags   = log.Ltime | log.Ldate | log.Lmicroseconds | log.Lshortfile
)

// A MessageHandler is called with the log level and message text.
type MessageHandler func(l LogLevel, msg string)

type Logger interface {
	AddHandler(level LogLevel, h MessageHandler)
	SetFlags(flag int)
	SetPrefix(prefix string)
	Debugln(vals ...interface{})
	Debugf(format string, vals ...interface{})
	Verboseln(vals ...interface{})
	Verbosef(format string, vals ...interface{})
	Infoln(vals ...interface{})
	Infof(format string, vals ...interface{})
	Warnln(vals ...interface{})
	Warnf(format string, vals ...interface{})
	ShouldDebug(facility string) bool
	SetDebug(facility string, enabled bool)
	Facilities() map[string]string
	FacilityDebugging() []string
	NewFacility(facility, description string) Logger
}

type logger struct {
	logger     *log.Logger
	handlers   [NumLevels][]MessageHandler
	facilities map[string]string   // facility name => description
	debug      map[string]struct{} // only facility names with debugging enabled
	traces     []string
	mut        sync.Mutex
}

// DefaultLogger logs to standard output with a time prefix.
var DefaultLogger = New()

func New() Logger {
	if os.Getenv("LOGGER_DISCARD") != "" {
		// Hack to completely disable logging, for example when running
		// benchmarks.
		return newLogger(io.Discard)
	}
	return newLogger(controlStripper{os.Stdout})
}

func newLogger(w io.Writer) Logger {
	traces := strings.FieldsFunc(os.Getenv("STTRACE"), func(r rune) bool {
		return strings.ContainsRune(",; ", r)
	})

	if len(traces) > 0 {
		if slices.Contains(traces, "all") {
			traces = []string{"all"}
		} else {
			slices.Sort(traces)
		}
	}

	return &logger{
		logger:     log.New(w, "", DefaultFlags),
		traces:     traces,
		facilities: make(map[string]string),
		debug:      make(map[string]struct{}),
	}
}

// AddHandler registers a new MessageHandler to receive messages with the
// specified log level or above.
func (l *logger) AddHandler(level LogLevel, h MessageHandler) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.handlers[level] = append(l.handlers[level], h)
}

// See log.SetFlags
func (l *logger) SetFlags(flag int) {
	l.logger.SetFlags(flag)
}

// See log.SetPrefix
func (l *logger) SetPrefix(prefix string) {
	l.logger.SetPrefix(prefix)
}

func (l *logger) callHandlers(level LogLevel, s string) {
	for ll := LevelDebug; ll <= level; ll++ {
		for _, h := range l.handlers[ll] {
			h(level, strings.TrimSpace(s))
		}
	}
}

// Debugln logs a line with a DEBUG prefix.
func (l *logger) Debugln(vals ...interface{}) {
	l.debugln(3, vals...)
}

func (l *logger) debugln(level int, vals ...interface{}) {
	s := fmt.Sprintln(vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(level, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *logger) Debugf(format string, vals ...interface{}) {
	l.debugf(3, format, vals...)
}

func (l *logger) debugf(level int, format string, vals ...interface{}) {
	s := fmt.Sprintf(format, vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(level, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

// Infoln logs a line with a VERBOSE prefix.
func (l *logger) Verboseln(vals ...interface{}) {
	s := fmt.Sprintln(vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(2, "VERBOSE: "+s)
	l.callHandlers(LevelVerbose, s)
}

// Infof logs a formatted line with a VERBOSE prefix.
func (l *logger) Verbosef(format string, vals ...interface{}) {
	s := fmt.Sprintf(format, vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(2, "VERBOSE: "+s)
	l.callHandlers(LevelVerbose, s)
}

// Infoln logs a line with an INFO prefix.
func (l *logger) Infoln(vals ...interface{}) {
	s := fmt.Sprintln(vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

// Infof logs a formatted line with an INFO prefix.
func (l *logger) Infof(format string, vals ...interface{}) {
	s := fmt.Sprintf(format, vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

// Warnln logs a formatted line with a WARNING prefix.
func (l *logger) Warnln(vals ...interface{}) {
	s := fmt.Sprintln(vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

// Warnf logs a formatted line with a WARNING prefix.
func (l *logger) Warnf(format string, vals ...interface{}) {
	s := fmt.Sprintf(format, vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

// ShouldDebug returns true if the given facility has debugging enabled.
func (l *logger) ShouldDebug(facility string) bool {
	l.mut.Lock()
	_, res := l.debug[facility]
	l.mut.Unlock()
	return res
}

// SetDebug enabled or disables debugging for the given facility name.
func (l *logger) SetDebug(facility string, enabled bool) {
	l.mut.Lock()
	defer l.mut.Unlock()
	if _, ok := l.debug[facility]; enabled && !ok {
		l.SetFlags(DebugFlags)
		l.debug[facility] = struct{}{}
	} else if !enabled && ok {
		delete(l.debug, facility)
		if len(l.debug) == 0 {
			l.SetFlags(DefaultFlags)
		}
	}
}

// isTraced returns whether the facility name is contained in STTRACE.
func (l *logger) isTraced(facility string) bool {
	if len(l.traces) > 0 {
		if l.traces[0] == "all" {
			return true
		}

		_, found := slices.BinarySearch(l.traces, facility)
		return found
	}

	return false
}

// FacilityDebugging returns the set of facilities that have debugging
// enabled.
func (l *logger) FacilityDebugging() []string {
	enabled := make([]string, 0, len(l.debug))
	l.mut.Lock()
	for facility := range l.debug {
		enabled = append(enabled, facility)
	}
	l.mut.Unlock()
	return enabled
}

// Facilities returns the currently known set of facilities and their
// descriptions.
func (l *logger) Facilities() map[string]string {
	l.mut.Lock()
	res := make(map[string]string, len(l.facilities))
	for facility, descr := range l.facilities {
		res[facility] = descr
	}
	l.mut.Unlock()
	return res
}

// NewFacility returns a new logger bound to the named facility.
func (l *logger) NewFacility(facility, description string) Logger {
	l.SetDebug(facility, l.isTraced(facility))

	l.mut.Lock()
	l.facilities[facility] = description
	l.mut.Unlock()

	return &facilityLogger{
		logger:   l,
		facility: facility,
	}
}

// A facilityLogger is a regular logger but bound to a facility name. The
// Debugln and Debugf methods are no-ops unless debugging has been enabled for
// this facility on the parent logger.
type facilityLogger struct {
	*logger
	facility string
}

// Debugln logs a line with a DEBUG prefix.
func (l *facilityLogger) Debugln(vals ...interface{}) {
	if !l.ShouldDebug(l.facility) {
		return
	}
	l.logger.debugln(3, vals...)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *facilityLogger) Debugf(format string, vals ...interface{}) {
	if !l.ShouldDebug(l.facility) {
		return
	}
	l.logger.debugf(3, format, vals...)
}

// A Recorder keeps a size limited record of log events.
type Recorder interface {
	Since(t time.Time) []Line
	Clear()
}

type recorder struct {
	lines   []Line
	initial int
	mut     sync.Mutex
}

// A Line represents a single log entry.
type Line struct {
	When    time.Time `json:"when"`
	Message string    `json:"message"`
	Level   LogLevel  `json:"level"`
}

func NewRecorder(l Logger, level LogLevel, size, initial int) Recorder {
	r := &recorder{
		lines:   make([]Line, 0, size),
		initial: initial,
	}
	l.AddHandler(level, r.append)
	return r
}

func (r *recorder) Since(t time.Time) []Line {
	r.mut.Lock()
	defer r.mut.Unlock()

	res := r.lines

	for i := 0; i < len(res); i++ {
		if res[i].When.After(t) {
			// We must copy the result as r.lines can be mutated as soon as the lock
			// is released.
			res = res[i:]
			cp := make([]Line, len(res))
			copy(cp, res)
			return cp
		}
	}
	return nil
}

func (r *recorder) Clear() {
	r.mut.Lock()
	r.lines = r.lines[:0]
	r.mut.Unlock()
}

func (r *recorder) append(l LogLevel, msg string) {
	line := Line{
		When:    time.Now(), // intentionally high precision
		Message: msg,
		Level:   l,
	}

	r.mut.Lock()
	defer r.mut.Unlock()

	if len(r.lines) == cap(r.lines) {
		if r.initial > 0 {
			// Shift all lines one step to the left, keeping the "initial" first intact.
			copy(r.lines[r.initial+1:], r.lines[r.initial+2:])
		} else {
			copy(r.lines, r.lines[1:])
		}
		// Add the new one at the end
		r.lines[len(r.lines)-1] = line
		return
	}

	r.lines = append(r.lines, line)
	if len(r.lines) == r.initial {
		r.lines = append(r.lines, Line{time.Now(), "...", l})
	}
}

// controlStripper is a Writer that replaces control characters
// with spaces.
type controlStripper struct {
	io.Writer
}

func (s controlStripper) Write(data []byte) (int, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			// Newlines are OK
			continue
		}
		if b < 32 {
			// Characters below 32 are control characters
			data[i] = ' '
		}
	}
	return s.Writer.Write(data)
}
