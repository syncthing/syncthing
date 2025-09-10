// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestFormattingHandler(t *testing.T) {
	buf := new(bytes.Buffer)
	h := &formattingHandler{
		opts: &formattingOptions{
			LineFormat:   DefaultLineFormat,
			out:          buf,
			timeOverride: time.Unix(1234567890, 0).In(time.UTC),
		},
	}

	l := slog.New(h).With("a", "a")
	l.Info("A basic info line", "attr1", "val with spaces", "attr2", 2, "attr3", `val"quote`)
	l.Info("A basic info line", "attr1", "paren)thesis")
	l.Info("An info line with an empty value", "attr1", "")
	l.Info("An info line with grouped values", "attr1", "val1", slog.Group("foo", "attr2", 2, slog.Group("bar", "attr3", "3")))

	l2 := l.WithGroup("foo")
	l2.Info("An info line with grouped values via logger", "attr1", "val1", "attr2", 2)

	l3 := l2.WithGroup("bar")
	l3.Info("An info line with nested grouped values via logger", "attr1", "val1", "attr2", 2)

	l3.Debug("A debug entry")
	l3.Warn("A warning entry")
	l3.Error("An error")

	exp := `
2009-02-13 23:31:30 INF A basic info line (attr1="val with spaces" attr2=2 attr3="val\"quote" a=a log.pkg=slogutil)
2009-02-13 23:31:30 INF A basic info line (attr1="paren)thesis" a=a log.pkg=slogutil)
2009-02-13 23:31:30 INF An info line with an empty value (attr1="" a=a log.pkg=slogutil)
2009-02-13 23:31:30 INF An info line with grouped values (attr1=val1 foo.attr2=2 foo.bar.attr3=3 a=a log.pkg=slogutil)
2009-02-13 23:31:30 INF An info line with grouped values via logger (foo.attr1=val1 foo.attr2=2 a=a log.pkg=slogutil)
2009-02-13 23:31:30 INF An info line with nested grouped values via logger (bar.foo.attr1=val1 bar.foo.attr2=2 a=a log.pkg=slogutil)
2009-02-13 23:31:30 WRN A warning entry (a=a log.pkg=slogutil)
2009-02-13 23:31:30 ERR An error (a=a log.pkg=slogutil)`

	if strings.TrimSpace(buf.String()) != strings.TrimSpace(exp) {
		t.Log(buf.String())
		t.Log(exp)
		t.Error("mismatch")
	}
}
