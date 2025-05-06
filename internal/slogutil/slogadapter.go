package slogutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/syncthing/syncthing/lib/logger"
)

var slogDef = slog.New(newLogHandler(slog.LevelInfo))

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
		pkgName := funcNameToPkg(fram.Function)
		packages[pkgName] = name
		return &adapter{slogDef.With("pkg", pkgName)}
	}
	return &adapter{slogDef}
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
func (a adapter) SetFlags(flag int)                                         {}
func (a adapter) SetPrefix(prefix string)                                   {}
func (a adapter) ShouldDebug(facility string) bool                          { return true }
func (a adapter) SetDebug(facility string, enabled bool)                    {}
func (a adapter) Facilities() map[string]string                             { return Packages() }
func (a adapter) FacilityDebugging() []string                               { return nil }
func (a adapter) NewFacility(facility, description string) logger.Logger    { return a }

func newLogHandler(level slog.Level) slog.Handler {
	const logFmt = "2006-01-02 15:04:05"
	color := isatty.IsTerminal(os.Stdout.Fd()) || os.Getenv("MONITOR_IS_STDOUT") != ""
	return tint.NewHandler(os.Stdout, &tint.Options{
		Level:      level,
		TimeFormat: logFmt,
		NoColor:    !color,
	})
}
