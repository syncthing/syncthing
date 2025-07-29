// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/config"
)

const (
	globalMigrationVersion = 1
	globalMigrationDBKey   = "globalMigrationVersion"
)

func globalMigration(kv db.KV, cfg config.Wrapper) error {
	miscDB := db.NewMiscDB(kv)
	prevVersion, _, err := miscDB.Int64(globalMigrationDBKey)
	if err != nil {
		return err
	}

	if prevVersion >= globalMigrationVersion {
		return nil
	}

	// currently no migrations

	return miscDB.PutInt64(globalMigrationDBKey, globalMigrationVersion)
}
