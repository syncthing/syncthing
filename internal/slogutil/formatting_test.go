// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"log/slog"
	"os"
	"time"
)

func ExampleFormattingHandler() {
	h := &FormattingHandler{
		out:          os.Stdout,
		timeOverride: time.Unix(1234567890, 0),
	}

	l := slog.New(h).With("a", "a")
	l.Info("outer message here", "attr1", "val with spaces", "attr2", 2)
	l.Info("outer message here", "attr2", 2, "attr1", "val with spaces")
	l.Info("outer message here", "attr1", "val1", slog.Group("foo", "attr2", 2, slog.Group("bar", "attr3", "3")))

	l2 := l.WithGroup("foo")
	l2.Info("foo group message here", "attr1", "val1", "attr2", 2)

	l3 := l2.WithGroup("bar")
	l3.Info("bar group message here", "attr1", "val1", "attr2", 2)
	l3.Info("bar group message here", "attr1", "val1", "attr2", 2, "attr1", "replaced")

	// Output:
	// 2009-02-14 00:31:30 INF outer message here attr1="val with spaces" attr2=2 a=a src.pkg=slogutil
	// 2009-02-14 00:31:30 INF outer message here attr2=2 attr1="val with spaces" a=a src.pkg=slogutil
	// 2009-02-14 00:31:30 INF outer message here attr1=val1 foo.attr2=2 foo.bar.attr3=3 a=a src.pkg=slogutil
	// 2009-02-14 00:31:30 INF foo group message here foo.attr1=val1 foo.attr2=2 a=a src.pkg=slogutil
	// 2009-02-14 00:31:30 INF bar group message here bar.foo.attr1=val1 bar.foo.attr2=2 a=a src.pkg=slogutil
	// 2009-02-14 00:31:30 INF bar group message here bar.foo.attr1=val1 bar.foo.attr2=2 bar.foo.attr1=replaced a=a src.pkg=slogutil
}
