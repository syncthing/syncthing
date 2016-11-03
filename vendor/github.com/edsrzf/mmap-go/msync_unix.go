// Copyright 2011 Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux openbsd solaris

package mmap

import (
	"syscall"
)

const _SYS_MSYNC = syscall.SYS_MSYNC
const _MS_SYNC = syscall.MS_SYNC
