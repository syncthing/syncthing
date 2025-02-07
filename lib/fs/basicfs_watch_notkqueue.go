// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !dragonfly && !freebsd && !netbsd && !openbsd && !kqueue && !ios
// +build !dragonfly,!freebsd,!netbsd,!openbsd,!kqueue,!ios

package fs

// WatchKqueue indicates if kqueue is used for filesystem watching
const WatchKqueue = false
