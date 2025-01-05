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
	DefaultFlags  = log.Ltime | log.Ldate
	DebugFlags    = log.Ltime | log.Ldate | log.Lmicroseconds | log.Lshortfile
	LevelDefault  = LevelInfo // The default logging level, if not specified.
	LevelDisabled = NumLevels // disables all logging, as it quiet even warnings.
	delimiters    = ",; \t"   // The characters allowed to delimit the facilities.
)

var levelPrefix = map[LogLevel]string{
	LevelDebug:   "DEBUG: ",
	LevelVerbose: "VERBOSE: ",
	LevelInfo:    "INFO: ",
	LevelWarn:    "WARN: ",
}

var levelMap = map[string]LogLevel{
	"debug":   LevelDebug,
	"verbose": LevelVerbose,
	"info":    LevelInfo,
	"warn":    LevelWarn,
	// start with a unique letter so abbreviation is unique.
	"off": LevelDisabled,
}

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
	facilities map[string]string // facility name => description
	mut        sync.Mutex
	levels     map[string]LogLevel // facility name and its logging level
	callLevel  int                 // so common code can be shared
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
	levels := parseSttrace()

	return &logger{
		logger:     log.New(w, "", DefaultFlags),
		facilities: make(map[string]string),
		levels:     levels,
		callLevel:  3,
	}
}

// parseSttrace parses an STTRACE environment variable in the form:
//
//	facility[:level][,facility2[:level2]] ...
//
// For example:
//
//	all:warn,fs:info,model
//
// logs everything at the WARN level (so no INFO lines in the logs),
// the fs facility at the INFO level (no DEBUG lines)
// and the model facility at the DEBUG level.
// Abbreviations are allowed, so
//
//	all:w,fs:i,model
//
// is the same as above.
func parseSttrace() map[string]LogLevel {
	sttrace := strings.ToLower(os.Getenv("STTRACE"))
	traces := strings.FieldsFunc(sttrace, func(r rune) bool {
		return strings.ContainsRune(delimiters, r)
	})

	levels := make(map[string]LogLevel, len(traces))
Next:
	for i, trace := range traces {
		parts := strings.Split(trace, ":")
		if len(parts) > 1 {
			trace = parts[0]
			lvl := parts[1]
			traces[i] = trace
			level, ok := levelMap[lvl]
			if ok {
				levels[trace] = level

				continue
			}
			for key, level := range levelMap {
				if strings.HasPrefix(key, lvl) {
					levels[trace] = level

					continue Next
				}
			}
			// if we get here, the user mistyped the level, but let's log at the
			// debug level anyway, since the user wants to log, we just don't
			// know at what level.
		}
		levels[trace] = LevelDebug
	}

	return levels
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

func (l *logger) log(level LogLevel, vals ...interface{}) {
	s := fmt.Sprintln(vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(l.callLevel, levelPrefix[level]+s)
	l.callHandlers(level, s)
}

func (l *logger) logf(level LogLevel, format string, vals ...interface{}) {
	s := fmt.Sprintf(format, vals...)
	l.mut.Lock()
	defer l.mut.Unlock()
	l.logger.Output(l.callLevel, levelPrefix[level]+s)
	l.callHandlers(level, s)
}

// Debugln logs a line with a DEBUG prefix.
func (l *logger) Debugln(vals ...interface{}) {
	l.log(LevelDebug, vals...)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *logger) Debugf(format string, vals ...interface{}) {
	l.logf(LevelDebug, format, vals...)
}

// Verboseln logs a line with a VERBOSE prefix.
func (l *logger) Verboseln(vals ...interface{}) {
	l.log(LevelVerbose, vals...)
}

// Verbosef logs a formatted line with a VERBOSE prefix.
func (l *logger) Verbosef(format string, vals ...interface{}) {
	l.logf(LevelVerbose, format, vals...)
}

// Infoln logs a line with an INFO prefix.
func (l *logger) Infoln(vals ...interface{}) {
	l.log(LevelInfo, vals...)
}

// Infof logs a formatted line with an INFO prefix.
func (l *logger) Infof(format string, vals ...interface{}) {
	l.logf(LevelInfo, format, vals...)
}

// Warnln logs a line with a WARNING prefix.
func (l *logger) Warnln(vals ...interface{}) {
	l.log(LevelWarn, vals...)
}

// Warnf logs a formatted line with a WARNING prefix.
func (l *logger) Warnf(format string, vals ...interface{}) {
	l.logf(LevelWarn, format, vals...)
}

// ShouldDebug returns true if facility is logging at the DEBUG level.
// For backwards compatibility, we don't look to see if the `all`
// facility is logging.
func (l *logger) ShouldDebug(facility string) bool {
	l.mut.Lock()
	level, ok := l.levels[facility]
	l.mut.Unlock()
	if ok {
		return level <= LevelDebug
	}

	return false
}

// IsEnabledFor returns true if facility (or "all") is logging at the
// logLevel log level.
func (l *logger) IsEnabledFor(facility string, logLevel LogLevel) bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	level, ok := l.levels[facility]
	if ok {
		return level <= logLevel
	}
	level, ok = l.levels["all"]
	if ok {
		return level <= logLevel
	}

	return false
}

// EffectiveLevel returns the level the facility is logging at. If logging
// is not enabled for the specified facility, the logging level for the
// "all" facility is returned, if it's enabled. Otherwise, LevelDefault
// (which is an alias for LevelInfo) is returned.
func (l *logger) EffectiveLevel(facility string) LogLevel {
	l.mut.Lock()
	defer l.mut.Unlock()
	level, ok := l.levels[facility]
	if ok {
		return level
	}
	level, ok = l.levels["all"]
	if ok {
		return level
	}

	return LevelDefault
}

// SetDebug enabled or disables DEBUG logging for the given facility name.
func (l *logger) SetDebug(facility string, enabled bool) {
	l.mut.Lock()
	defer l.mut.Unlock()
	if enabled {
		l.levels[facility] = LevelDebug
	} else {
		delete(l.levels, facility)
	}
	l.setLoggerFlags()
}

// setLoggerFlags set the underlying logger's flags to DebugFlags if any facility
// is logging at the DEBUG level.
func (l *logger) setLoggerFlags() {
	for _, level := range l.levels {
		if level <= LevelDebug {
			l.SetFlags(DebugFlags)

			return
		}
	}
	l.SetFlags(DefaultFlags)
}

// FacilityDebugging returns the set of facilities logging at the DEBUG level.
func (l *logger) FacilityDebugging() []string {
	enabled := make([]string, 0, len(l.levels))
	l.mut.Lock()
	for facility, level := range l.levels {
		if level <= LevelDebug {
			enabled = append(enabled, facility)
		}
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
	l.mut.Lock()
	l.facilities[facility] = description
	l.callLevel = 4
	l.setLoggerFlags()
	l.mut.Unlock()

	return &facilityLogger{
		logger:   l,
		facility: facility,
	}
}

// A facilityLogger is a regular logger but bound to a facility name. The
// Debugln and Debugf methods are no-ops unless the facility is logging at the
// DEBUG level on the parent logger. The Infoln and Infof methods are also
// no-ops unless the facility is logging at the INFO level on the parent logger.
// Similarly with the WARN level. At the ERROR level no logging occurs.
type facilityLogger struct {
	*logger
	facility string
}

func (l *facilityLogger) shouldLog(facility string, logLevel LogLevel) bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	level, ok := l.levels[facility]
	if ok {
		return logLevel >= level
	}
	level, ok = l.levels["all"]
	if ok {
		return logLevel >= level
	}

	return logLevel >= LevelDefault
}

// Debugln logs a line with a DEBUG prefix.
func (l *facilityLogger) Debugln(vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelDebug) {
		return
	}
	l.logger.Debugln(vals...)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *facilityLogger) Debugf(format string, vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelDebug) {
		return
	}
	l.logger.Debugf(format, vals...)
}

// Verboseln logs a line with a VERBOSE prefix.
func (l *facilityLogger) Verboseln(vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelVerbose) {
		return
	}
	l.logger.Verboseln(vals...)
}

// Verbosef logs a formatted line with a VERBOSE prefix.
func (l *facilityLogger) Verbosef(format string, vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelVerbose) {
		return
	}
	l.logger.Verbosef(format, vals...)
}

// Infoln logs a line with an INFO prefix.
func (l *facilityLogger) Infoln(vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelInfo) {
		return
	}
	l.logger.Infoln(vals...)
}

// Infof logs a formatted line with an INFO prefix.
func (l *facilityLogger) Infof(format string, vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelInfo) {
		return
	}
	l.logger.Infof(format, vals...)
}

// Warnln logs a formatted line with a WARNING prefix.
func (l *facilityLogger) Warnln(vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelWarn) {
		return
	}
	l.logger.Warnln(vals...)
}

// Warnf logs a formatted line with a WARNING prefix.
func (l *facilityLogger) Warnf(format string, vals ...interface{}) {
	if !l.shouldLog(l.facility, LevelWarn) {
		return
	}
	l.logger.Warnf(format, vals...)
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
