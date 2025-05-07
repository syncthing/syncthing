package slogutil

import (
	"context"
	"log/slog"
	"maps"
	"os"
	"strings"
	"sync"
)

var Levels = &LevelTracker{
	levels: make(map[string]slog.Level),
	descrs: make(map[string]string),
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
		Levels.Set(pkg, level)
	}
}

type LevelTracker struct {
	mut      sync.RWMutex
	defLevel slog.Level
	descrs   map[string]string     // package name to description
	levels   map[string]slog.Level // package name to level
}

func (t *LevelTracker) Get(pkg string) slog.Level {
	t.mut.RLock()
	defer t.mut.RUnlock()
	if level, ok := t.levels[pkg]; ok {
		return level
	}
	return t.defLevel
}

func (t *LevelTracker) Set(pkg string, level slog.Level) {
	t.mut.Lock()
	t.levels[pkg] = level
	t.mut.Unlock()
}

func (t *LevelTracker) SetDefault(level slog.Level) {
	t.mut.Lock()
	t.defLevel = level
	t.mut.Unlock()
}

func (t *LevelTracker) SetDescr(pkg, descr string) {
	t.mut.Lock()
	t.descrs[pkg] = descr
	t.mut.Unlock()
}

func (t *LevelTracker) Descrs() map[string]string {
	t.mut.RLock()
	defer t.mut.RUnlock()
	m := make(map[string]string, len(t.descrs))
	maps.Copy(m, t.descrs)
	return m
}

func (t *LevelTracker) Levels() map[string]slog.Level {
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

type PackageLeveler struct {
	t   *LevelTracker
	pkg string
}

func (t *LevelTracker) NewPackageLeveler(pkg string) slog.Leveler {
	return &PackageLeveler{t: t, pkg: pkg}
}

func (p *PackageLeveler) Level() slog.Level {
	return p.t.Get(p.pkg)
}

type LevelTrackingHandler struct {
	slog.Handler
	pkg string
}

func (l *LevelTrackingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return Levels.Get(l.pkg) <= level
}

func (l *LevelTrackingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LevelTrackingHandler{
		Handler: l.Handler.WithAttrs(attrs),
		pkg:     l.pkg,
	}
}

func (l *LevelTrackingHandler) WithGroup(name string) slog.Handler {
	return &LevelTrackingHandler{
		Handler: l.Handler.WithGroup(name),
		pkg:     l.pkg,
	}
}
