// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"log/slog"
	"os"
	"testing"
)

func TestFormattingHandler(t *testing.T) {
	h := &formattingHandler{
		out: os.Stdout,
	}
	l := slog.New(h).With("a", "a")
	l.Info("Message here", "attr1", "val with spaces", "attr2", 2)
	l.Info("Message here", "attr2", 2, "attr1", "val with spaces")
	l.Info("Message here", "attr1", "val1", slog.Group("foo", "attr2", 2, slog.Group("bar", "attr3", "3")))
	l2 := l.WithGroup("foo")
	l2.Info("Message here", "attr1", "val1", "attr2", 2)
	l3 := l2.WithGroup("bar")
	l3.Info("Message here", "attr1", "val1", "attr2", 2)
	l3.Info("Message here", "attr1", "val1", "attr2", 2, "attr1", "replaced")
}
