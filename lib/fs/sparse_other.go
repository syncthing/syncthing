// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows && !linux && !darwin && !freebsd

package fs

// SetSparse is unsupported on this platform.
func SetSparse(File) {}

// NextHole is unsupported on this platform (no exposed hole-detection API such
// as SEEK_HOLE), so the caller falls back to reusing nothing.
func NextHole(File, int64) (int64, bool) { return 0, false }
