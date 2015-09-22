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

type Logger struct {
	logger   *log.Logger
	handlers [NumLevels][]MessageHandler
	mut      sync.Mutex
}

// The default logger logs to standard output with a time prefix.
var DefaultLogger = New()

func New() *Logger {
	if os.Getenv("LOGGER_DISCARD") != "" {
		// Hack to completely disable logging, for example when running benchmarks.
		return &Logger{
			logger: log.New(ioutil.Discard, "", 0),
		}
	}

	return &Logger{
		logger: log.New(os.Stdout, "", log.Ltime),
	}
}

// AddHandler registers a new MessageHandler to receive messages with the
// specified log level or above.
func (l *Logger) AddHandler(level LogLevel, h MessageHandler) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.handlers[level] = append(l.handlers[level], h)
}

// See log.SetFlags
func (l *Logger) SetFlags(flag int) {
	l.logger.SetFlags(flag)
}

// See log.SetPrefix
func (l *Logger) SetPrefix(prefix string) {
	l.logger.SetPrefix(prefix)
}

func (l *Logger) callHandlers(level LogLevel, s string) {
	for _, h := range l.handlers[level] {
		h(level, strings.TrimSpace(s))
	}
}

// Debugln logs a line with a DEBUG prefix.
func (l *Logger) Debugln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

// Debugf logs a formatted line with a DEBUG prefix.
func (l *Logger) Debugf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

// Infoln logs a line with a VERBOSE prefix.
func (l *Logger) Verboseln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "VERBOSE: "+s)
	l.callHandlers(LevelVerbose, s)
}

// Infof logs a formatted line with a VERBOSE prefix.
func (l *Logger) Verbosef(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "VERBOSE: "+s)
	l.callHandlers(LevelVerbose, s)
}

// Infoln logs a line with an INFO prefix.
func (l *Logger) Infoln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

// Infof logs a formatted line with an INFO prefix.
func (l *Logger) Infof(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

// Okln logs a line with an OK prefix.
func (l *Logger) Okln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "OK: "+s)
	l.callHandlers(LevelOK, s)
}

// Okf logs a formatted line with an OK prefix.
func (l *Logger) Okf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "OK: "+s)
	l.callHandlers(LevelOK, s)
}

// Warnln logs a formatted line with a WARNING prefix.
func (l *Logger) Warnln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

// Warnf logs a formatted line with a WARNING prefix.
func (l *Logger) Warnf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

// Fatalln logs a line with a FATAL prefix and exits the process with exit
// code 1.
func (l *Logger) Fatalln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "FATAL: "+s)
	l.callHandlers(LevelFatal, s)
	os.Exit(1)
}

// Fatalf logs a formatted line with a FATAL prefix and exits the process with
// exit code 1.
func (l *Logger) Fatalf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "FATAL: "+s)
	l.callHandlers(LevelFatal, s)
	os.Exit(1)
}
