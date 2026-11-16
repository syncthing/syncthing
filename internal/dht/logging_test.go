// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/gologshim"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLibraryLogCore(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	core := libraryLogCore{}
	logger := zap.New(core).Named("swarm2").With(zap.String("peer", "test-peer"))
	logger.Debug("dial failed", zap.Error(errors.New("dial backoff: nested error")))
	for _, want := range []string{"level=DEBUG", "libp2pSystem=swarm2", "peer=test-peer", "dial backoff: nested error"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
	// With must not mutate the original core's fields.
	if len(core.fields) != 0 {
		t.Fatal("With mutated original core")
	}
	output.Reset()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	logger.Debug("filtered")
	logger.Warn("bootstrap failed")
	if strings.Contains(output.String(), "filtered") || !strings.Contains(output.String(), "bootstrap failed") {
		t.Fatalf("unexpected level filtering: %s", output.String())
	}
	if libraryLogLevel(zapcore.InfoLevel) != slog.LevelDebug {
		t.Fatal("verbose library info should require debug logging")
	}
}

// libp2p's native slog loggers need an explicit bridge in addition to the
// older DHT dependency's Zap loggers.
func TestLibraryLogShim(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	setupLibraryLogging()
	gologshim.Logger("autonat").Debug("test dialback result", "peer", "test-peer", "err", errors.New("dial failed"))
	for _, want := range []string{"test dialback result", "test-peer", "dial failed", "autonat"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
}
