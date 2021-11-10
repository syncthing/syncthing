// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build noassets
// +build noassets

package auto

import "github.com/syncthing/syncthing/lib/assets"

func Assets() map[string]assets.Asset {
	return nil
}
