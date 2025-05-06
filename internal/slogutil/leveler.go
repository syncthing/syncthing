package slogutil

import (
	"context"
	"log/slog"
	"sync"
)

var Levels = &LevelTracker{levels: make(map[string]slog.Level)}

type LevelTracker struct {
	mut    sync.RWMutex
	levels map[string]slog.Level // package name to level
}

func (t *LevelTracker) Get(pkg string) slog.Level {
	t.mut.RLock()
	defer t.mut.RUnlock()
	return t.levels[pkg]
}

func (t *LevelTracker) Set(pkg string, level slog.Level) {
	t.mut.Lock()
	t.levels[pkg] = level
	t.mut.Unlock()
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
