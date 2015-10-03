// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

// Package logger implements a standardized logger with callback functionality
package logger

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelVerbose
	LevelInfo
	LevelOK
	LevelWarn
	LevelFatal
	NumLevels
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
	Okln(vals ...interface{})
	Okf(format string, vals ...interface{})
	Warnln(vals ...interface{})
	Warnf(format string, vals ...interface{})
	Fatalln(vals ...interface{})
	Fatalf(format string, vals ...interface{})
	ShouldDebug(facility string) bool
	SetDebug(facility string, enabled bool)
	Facilities() (enabled, disabled []string)
	NewFacility(facility string) Logger
}

type logger struct {
	logger   *log.Logger
	handlers [NumLevels][]MessageHandler
	debug    map[string]bool
	mut      sync.Mutex
}

// The default logger logs to standard output with a time prefix.
var DefaultLogger = New()

func New() Logger {
	if os.Getenv("LOGGER_DISCARD") != "" {
		// Hack to completely disable logging, for example when running benchmarks.
		return &logger{
			logger: log.New(ioutil.Discard, "", 0),
		}
	}

	return &logger{
		logger: log.New(os.Stdout, "", log.Ltime),
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
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *logger) Debugf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

// Infoln logs a line with a VERBOSE prefix.
func (l *logger) Verboseln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "VERBOSE: "+s)
	l.callHandlers(LevelVerbose, s)
}

// Infof logs a formatted line with a VERBOSE prefix.
func (l *logger) Verbosef(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "VERBOSE: "+s)
	l.callHandlers(LevelVerbose, s)
}

// Infoln logs a line with an INFO prefix.
func (l *logger) Infoln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

// Infof logs a formatted line with an INFO prefix.
func (l *logger) Infof(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

// Okln logs a line with an OK prefix.
func (l *logger) Okln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "OK: "+s)
	l.callHandlers(LevelOK, s)
}

// Okf logs a formatted line with an OK prefix.
func (l *logger) Okf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "OK: "+s)
	l.callHandlers(LevelOK, s)
}

// Warnln logs a formatted line with a WARNING prefix.
func (l *logger) Warnln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

// Warnf logs a formatted line with a WARNING prefix.
func (l *logger) Warnf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

// Fatalln logs a line with a FATAL prefix and exits the process with exit
// code 1.
func (l *logger) Fatalln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "FATAL: "+s)
	l.callHandlers(LevelFatal, s)
	os.Exit(1)
}

// Fatalf logs a formatted line with a FATAL prefix and exits the process with
// exit code 1.
func (l *logger) Fatalf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "FATAL: "+s)
	l.callHandlers(LevelFatal, s)
	os.Exit(1)
}

// ShouldDebug returns true if the given facility has debugging enabled.
func (l *logger) ShouldDebug(facility string) bool {
	l.mut.Lock()
	res := l.debug[facility]
	l.mut.Unlock()
	return res
}

// SetDebug enabled or disables debugging for the given facility name.
func (l *logger) SetDebug(facility string, enabled bool) {
	l.mut.Lock()
	l.debug[facility] = enabled
	l.mut.Unlock()
}

// Facilities returns the currently known set of facilities, both those for
// which debug is enabled and those for which it is disabled.
func (l *logger) Facilities() (enabled, disabled []string) {
	l.mut.Lock()
	for facility, isEnabled := range l.debug {
		if isEnabled {
			enabled = append(enabled, facility)
		} else {
			disabled = append(disabled, facility)
		}
	}
	l.mut.Unlock()
	return
}

// NewFacility returns a new logger bound to the named facility.
func (l *logger) NewFacility(facility string) Logger {
	l.mut.Lock()
	if l.debug == nil {
		l.debug = make(map[string]bool)
	}
	l.debug[facility] = false
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
	l.logger.Debugln(vals...)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *facilityLogger) Debugf(format string, vals ...interface{}) {
	if !l.ShouldDebug(l.facility) {
		return
	}
	l.logger.Debugf(format, vals...)
}
