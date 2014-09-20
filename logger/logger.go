// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package logger implements a standardized logger with callback functionality
package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelOK
	LevelWarn
	LevelFatal
	NumLevels
)

type MessageHandler func(l LogLevel, msg string)

type Logger struct {
	logger   *log.Logger
	handlers [NumLevels][]MessageHandler
	mut      sync.Mutex
}

var DefaultLogger = New()

func New() *Logger {
	return &Logger{
		logger: log.New(os.Stdout, "", log.Ltime),
	}
}

func (l *Logger) AddHandler(level LogLevel, h MessageHandler) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.handlers[level] = append(l.handlers[level], h)
}

func (l *Logger) SetFlags(flag int) {
	l.logger.SetFlags(flag)
}

func (l *Logger) SetPrefix(prefix string) {
	l.logger.SetPrefix(prefix)
}

func (l *Logger) callHandlers(level LogLevel, s string) {
	for _, h := range l.handlers[level] {
		h(level, strings.TrimSpace(s))
	}
}

func (l *Logger) Debugln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}

func (l *Logger) Debugf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "DEBUG: "+s)
	l.callHandlers(LevelDebug, s)
}
func (l *Logger) Infoln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

func (l *Logger) Infof(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "INFO: "+s)
	l.callHandlers(LevelInfo, s)
}

func (l *Logger) Okln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "OK: "+s)
	l.callHandlers(LevelOK, s)
}

func (l *Logger) Okf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "OK: "+s)
	l.callHandlers(LevelOK, s)
}

func (l *Logger) Warnln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

func (l *Logger) Warnf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "WARNING: "+s)
	l.callHandlers(LevelWarn, s)
}

func (l *Logger) Fatalln(vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintln(vals...)
	l.logger.Output(2, "FATAL: "+s)
	l.callHandlers(LevelFatal, s)
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, vals ...interface{}) {
	l.mut.Lock()
	defer l.mut.Unlock()
	s := fmt.Sprintf(format, vals...)
	l.logger.Output(2, "FATAL: "+s)
	l.callHandlers(LevelFatal, s)
	os.Exit(1)
}
