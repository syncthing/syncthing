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

var slogDef = newLogHandler()

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
		pkgName := funcNameToPkg(fram.Function)
		globalLevels.SetDescr(pkgName, descr)
		h := &levelTrackingHandler{
			Handler: slogDef,
			levels:  globalLevels,
			pkg:     pkgName,
		}
		return &adapter{slog.New(h).With("pkg", pkgName)}
	}
	return &adapter{slog.New(slogDef)}
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
	h := a.Logger.Handler()
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
func (a adapter) ShouldDebug(facility string) bool {
	return globalLevels.Get(facility) >= slog.LevelDebug
}

func newLogHandler() slog.Handler {
	return &recordingHandler{}
	// const logFmt = "2006-01-02 15:04:05"
	// return tint.NewHandler(os.Stdout, &tint.Options{
	// 	TimeFormat: logFmt,
	// 	NoColor:    true,
	// })
}
