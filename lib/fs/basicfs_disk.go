// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !ios

package fs

import (
	"github.com/shirou/gopsutil/disk"
)

func DiskUsage(name string) (Usage, error) {
	return disk.usage(name)
}
