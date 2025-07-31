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
		out:          buf,
		timeOverride: time.Unix(1234567890, 0).In(time.UTC),
	}

	l := slog.New(h).With("a", "a")
	l.Info("outer message here", "attr1", "val with spaces", "attr2", 2, "attr3", `val"quote`)
	l.Info("outer message here", "attr2", 2, "attr1", "val with spaces")
	l.Info("outer message here", "attr1", "val1", slog.Group("foo", "attr2", 2, slog.Group("bar", "attr3", "3")))

	l2 := l.WithGroup("foo")
	l2.Info("foo group message here", "attr1", "val1", "attr2", 2)

	l3 := l2.WithGroup("bar")
	l3.Info("bar group message here", "attr1", "val1", "attr2", 2)
	l3.Info("bar group message here", "attr1", "val1", "attr2", 2, "attr1", "replaced")

	exp := `
2009-02-13 23:31:30 INF outer message here (attr1="val with spaces" attr2=2 attr3="val\"quote" a=a src.pkg=slogutil)
2009-02-13 23:31:30 INF outer message here (attr2=2 attr1="val with spaces" a=a src.pkg=slogutil)
2009-02-13 23:31:30 INF outer message here (attr1=val1 foo.attr2=2 foo.bar.attr3=3 a=a src.pkg=slogutil)
2009-02-13 23:31:30 INF foo group message here (foo.attr1=val1 foo.attr2=2 a=a src.pkg=slogutil)
2009-02-13 23:31:30 INF bar group message here (bar.foo.attr1=val1 bar.foo.attr2=2 a=a src.pkg=slogutil)
2009-02-13 23:31:30 INF bar group message here (bar.foo.attr1=val1 bar.foo.attr2=2 bar.foo.attr1=replaced a=a src.pkg=slogutil)`

	if strings.TrimSpace(buf.String()) != strings.TrimSpace(exp) {
		t.Log(buf.String())
		t.Log(exp)
		t.Error("mismatch")
	}
}
