// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build race
// +build race

package build

func init() {
	if Tags == "" {
		Tags = "race"
	} else {
		Tags = Tags + ",race"
	}
}
