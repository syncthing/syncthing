// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !linux,!windows

package fs

var copyRangeTests = []copyRangeTestScenario{
	{
		name:        "generic",
		copyFn:      copyRangeGeneric,
		mustSucceed: true,
	},
	{
		name:        "ioctl",
		copyFn:      wrapOptimised(copyRangeIoctl),
		mustSucceed: false,
	},
	{
		name:        "sendfile",
		copyFn:      wrapOptimised(copyFileSendFile),
		mustSucceed: false,
	},
}
