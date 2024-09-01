// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import "github.com/cockroachdb/pebble"

func OpenPebbleDB(location string) (Backend, error) {
	db, err := pebble.Open(location, nil)
	if err != nil {
		return nil, err
	}
	return newPebbleBackend(db, location), nil
}
