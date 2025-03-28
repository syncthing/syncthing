// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"embed"
	"io/fs"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

const currentSchemaVersion = 1

//go:embed sql/**
var embedded embed.FS

func (s *DB) runScripts(glob string, filter ...func(s string) bool) error {
	scripts, err := fs.Glob(embedded, glob)
	if err != nil {
		return wrap(err)
	}

	tx, err := s.sql.Begin()
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck

nextScript:
	for _, scr := range scripts {
		for _, fn := range filter {
			if !fn(scr) {
				l.Debugln("Skipping script", scr)
				continue nextScript
			}
		}
		l.Debugln("Executing script", scr)
		bs, err := fs.ReadFile(embedded, scr)
		if err != nil {
			return wrap(err, scr)
		}
		// SQLite requires one statement per exec, so we split the init
		// files on lines containing only a semicolon and execute them
		// separately. We require it on a separate line because there are
		// also statement-internal semicolons in the triggers.
		for _, stmt := range strings.Split(string(bs), "\n;") {
			if _, err := tx.Exec(stmt); err != nil {
				return wrap(err, stmt)
			}
		}
	}

	return wrap(tx.Commit())
}

type schemaVersion struct {
	SchemaVersion    int
	AppliedAt        int64
	SyncthingVersion string
}

func (s *schemaVersion) AppliedTime() time.Time {
	return time.Unix(0, s.AppliedAt)
}

func (s *DB) setAppliedSchemaVersion(ver int) error {
	_, err := s.stmt(`
		INSERT OR IGNORE INTO schemamigrations (schema_version, applied_at, syncthing_version)
		VALUES (?, ?, ?)
	`).Exec(ver, time.Now().UnixNano(), build.LongVersion)
	return wrap(err)
}

func (s *DB) getAppliedSchemaVersion() (schemaVersion, error) {
	var v schemaVersion
	err := s.stmt(`
		SELECT schema_version as schemaversion, applied_at as appliedat, syncthing_version as syncthingversion FROM schemamigrations
		ORDER BY schema_version DESC
		LIMIT 1
	`).Get(&v)
	return v, wrap(err)
}
