package slogutil

import (
	"context"
	"log/slog"
	"maps"
	"os"
	"strings"
	"sync"
)

// A levelTracker keeps track of log level per package. This enables the
// traditional STTRACE variable to set certain packages to debug level, but
// also allows setting packages to other levels such as WARN to silence
// INFO-level messages.
//
// The STTRACE environment variable is one way of controlling this, where
// mentioning a package makes it DEBUG level:
//     STTRACE="model,protocol"  # model and protocol are at DEBUG level
// however you can also give specific levels after a colon:
//     STTRACE="model:WARNING,protocol:DEBUG"

var globalLevels = &levelTracker{
	levels: make(map[string]slog.Level),
	descrs: make(map[string]string),
}

func PackageDescrs() map[string]string {
	return globalLevels.Descrs()
}

func PackageLevels() map[string]slog.Level {
	return globalLevels.Levels()
}

func SetPackageLevel(pkg string, level slog.Level) {
	globalLevels.Set(pkg, level)
}

func SetDefaultLevel(level slog.Level) {
	globalLevels.SetDefault(level)
}

func init() {
	// Handle legacy STTRACE var
	pkgs := strings.Split(os.Getenv("STTRACE"), ",")
	for _, pkg := range pkgs {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		level := slog.LevelDebug
		if cutPkg, levelStr, ok := strings.Cut(pkg, ":"); ok {
			pkg = cutPkg
			if err := level.UnmarshalText([]byte(levelStr)); err != nil {
				slog.New(slogDef).Warn("Bad log level requested in STTRACE", "pkg", pkg, "level", levelStr, "error", err)
			}
		}
		globalLevels.Set(pkg, level)
	}
}

type levelTracker struct {
	mut      sync.RWMutex
	defLevel slog.Level
	descrs   map[string]string     // package name to description
	levels   map[string]slog.Level // package name to level
}

func (t *levelTracker) Get(pkg string) slog.Level {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if level, ok := t.levels[pkg]; ok {
		return level
	}
	return t.defLevel
}

func (t *levelTracker) Set(pkg string, level slog.Level) {
	t.mut.Lock()
	t.levels[pkg] = level
	t.mut.Unlock()
}

func (t *levelTracker) SetDefault(level slog.Level) {
	t.mut.Lock()
	t.defLevel = level
	t.mut.Unlock()
}

func (t *levelTracker) SetDescr(pkg, descr string) {
	t.mut.Lock()
	t.descrs[pkg] = descr
	t.mut.Unlock()
}

func (t *levelTracker) Descrs() map[string]string {
	t.mut.RLock()
	defer t.mut.RUnlock()
	m := make(map[string]string, len(t.descrs))
	maps.Copy(m, t.descrs)
	return m
}

func (t *levelTracker) Levels() map[string]slog.Level {
	t.mut.RLock()
	defer t.mut.RUnlock()
	m := make(map[string]slog.Level, len(t.descrs))
	for pkg := range t.descrs {
		if level, ok := t.levels[pkg]; ok {
			m[pkg] = level
		} else {
			m[pkg] = t.defLevel
		}
	}
	return m
}

type levelTrackingHandler struct {
	slog.Handler

	levels *levelTracker
	pkg    string
}

func (l *levelTrackingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return l.levels.Get(l.pkg) <= level
}

func (l *levelTrackingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelTrackingHandler{
		Handler: l.Handler.WithAttrs(attrs),
		levels:  l.levels,
		pkg:     l.pkg,
	}
}

func (l *levelTrackingHandler) WithGroup(name string) slog.Handler {
	return &levelTrackingHandler{
		Handler: l.Handler.WithGroup(name),
		levels:  l.levels,
		pkg:     l.pkg,
	}
}
