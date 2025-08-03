// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package slogutil

import (
	"log/slog"
)

// Expensive wraps a log value that is expensive to compute and should only
// be called if the log line is actually emitted.
func Expensive(fn func() any) expensive {
	return expensive{fn}
}

type expensive struct {
	fn func() any
}

func (e expensive) LogValue() slog.Value {
	return slog.AnyValue(e.fn())
}
