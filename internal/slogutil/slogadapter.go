package slogutil

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/logger"
)

var packages = make(map[string]string)

func Packages() map[string]string {
	return packages
}

func NewAdapter(name string) *adapter {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	pc := pcs[0]
	fr := runtime.CallersFrames([]uintptr{pc})
	if fram, _ := fr.Next(); fram.Function != "" {
		packages[funcNameToPkg(fram.Function)] = name
	}
	return &adapter{}
}

type adapter struct{}

func (a adapter) Debugln(vals ...interface{}) {
	log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelDebug)
}

func (a adapter) Debugf(format string, vals ...interface{}) {
	log(fmt.Sprintf(format, vals...), slog.LevelDebug)
}

func (a adapter) Verboseln(vals ...interface{}) {
	log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelInfo)
}

func (a adapter) Verbosef(format string, vals ...interface{}) {
	log(fmt.Sprintf(format, vals...), slog.LevelInfo)
}

func (a adapter) Infoln(vals ...interface{}) {
	log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelInfo)
}

func (a adapter) Infof(format string, vals ...interface{}) {
	log(fmt.Sprintf(format, vals...), slog.LevelInfo)
}

func (a adapter) Warnln(vals ...interface{}) {
	log(strings.TrimSpace(fmt.Sprintln(vals...)), slog.LevelError)
}

func (a adapter) Warnf(format string, vals ...interface{}) {
	log(fmt.Sprintf(format, vals...), slog.LevelError)
}

func log(msg string, level slog.Level) {
	h := slog.Default().Handler()
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

func (a adapter) AddHandler(level logger.LogLevel, h logger.MessageHandler) {}
func (a adapter) SetFlags(flag int)                                         {}
func (a adapter) SetPrefix(prefix string)                                   {}
func (a adapter) ShouldDebug(facility string) bool                          { return true }
func (a adapter) SetDebug(facility string, enabled bool)                    {}
func (a adapter) Facilities() map[string]string                             { return Packages() }
func (a adapter) FacilityDebugging() []string                               { return nil }
func (a adapter) NewFacility(facility, description string) logger.Logger    { return a }
