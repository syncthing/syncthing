// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"os"

	"github.com/cockroachdb/pebble"
)

func OpenPebble(location string) (Backend, error) {
	return openPebble(location, false)
}

// OpenPebbleTemporary creates a db in temporary storage and cleans up when closed
func OpenPebbleTemporary() (Backend, error) {
	path, err := os.MkdirTemp("", "pebble-db-temp-")
	if err != nil {
		return nil, err
	}
	return openPebble(path, true)
}

func openPebble(location string, temporary bool) (Backend, error) {
	db, err := pebble.Open(location, nil)
	if err != nil {
		return nil, err
	}
	return newPebbleBackend(db, location, temporary), nil
}
