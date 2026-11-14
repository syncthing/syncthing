// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	"log/slog"
	"sync"

	logging "github.com/ipfs/go-log/v2"
	"github.com/syncthing/syncthing/internal/slogutil"
	"go.uber.org/zap/zapcore"
)

func init() { slogutil.RegisterPackage("Peer-to-peer discovery DHT") }

var setupLibraryLoggingOnce sync.Once

func setupLibraryLogging() {
	setupLibraryLoggingOnce.Do(func() {
		// Include nested swarm dial errors and DHT bootstrap/refresh diagnostics in
		// the normal Syncthing log stream and support bundles. The slog handler
		// still controls whether debug records are emitted.
		logging.SetPrimaryCore(libraryLogCore{})
		for _, system := range []string{"dht", "dht/RtRefreshManager", "swarm2"} {
			_ = logging.SetLogLevel(system, "debug")
		}
	})
}

type libraryLogCore struct{ fields []zapcore.Field }

func (c libraryLogCore) Enabled(level zapcore.Level) bool {
	return slog.Default().Enabled(context.Background(), libraryLogLevel(level))
}
func (c libraryLogCore) With(fields []zapcore.Field) zapcore.Core {
	c.fields = append(append([]zapcore.Field(nil), c.fields...), fields...)
	return c
}
func (c libraryLogCore) Check(e zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return checked.AddCore(e, c)
	}
	return checked
}
func (c libraryLogCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range c.fields {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}
	attrs := []slog.Attr{slog.String("libp2pSystem", e.LoggerName)}
	for k, v := range enc.Fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	slog.LogAttrs(context.Background(), libraryLogLevel(e.Level), e.Message, attrs...)
	return nil
}
func (libraryLogCore) Sync() error { return nil }
func libraryLogLevel(level zapcore.Level) slog.Level {
	switch {
	case level < zapcore.InfoLevel:
		return slog.LevelDebug
	case level == zapcore.InfoLevel:
		return slog.LevelDebug
	case level == zapcore.WarnLevel:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
