// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ext

type ExtCallback interface {
	ExtCheckAvailableSpace(req uint64) bool
	ExtAccessPath(path string) string
}

var (
	Callback ExtCallback
)
