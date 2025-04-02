// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
)

const maxDBConns = 128

func Open(path string) (*DB, error) {
	// Open the database with options to enable foreign keys and recursive
	// triggers (needed for the delete+insert triggers on row replace).
	sqlDB, err := sqlx.Open(dbDriver, "file:"+path+"?"+commonOptions)
	if err != nil {
		return nil, wrap(err)
	}
	sqlDB.SetMaxOpenConns(maxDBConns)
	if _, err := sqlDB.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return nil, wrap(err, "PRAGMA journal_mode")
	}
	if _, err := sqlDB.Exec(`PRAGMA optimize = 0x10002`); err != nil {
		// https://www.sqlite.org/pragma.html#pragma_optimize
		return nil, wrap(err, "PRAGMA optimize")
	}
	if _, err := sqlDB.Exec(`PRAGMA journal_size_limit = 67108864`); err != nil {
		// https://www.powersync.com/blog/sqlite-optimizations-for-ultra-high-performance
		return nil, wrap(err, "PRAGMA journal_size_limit")
	}
	return openCommon(sqlDB)
}

// Open the database with options suitable for the migration inserts. This
// is not a safe mode of operation for normal processing, use only for bulk
// inserts with a close afterwards.
func OpenForMigration(path string) (*DB, error) {
	sqlDB, err := sqlx.Open(dbDriver, "file:"+path+"?"+commonOptions)
	if err != nil {
		return nil, wrap(err, "open")
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = 0`); err != nil {
		return nil, wrap(err, "PRAGMA foreign_keys")
	}
	if _, err := sqlDB.Exec(`PRAGMA journal_mode = OFF`); err != nil {
		return nil, wrap(err, "PRAGMA journal_mode")
	}
	if _, err := sqlDB.Exec(`PRAGMA synchronous = 0`); err != nil {
		return nil, wrap(err, "PRAGMA synchronous")
	}
	return openCommon(sqlDB)
}

func OpenTemp() (*DB, error) {
	// SQLite has a memory mode, but it works differently with concurrency
	// compared to what we need with the WAL mode. So, no memory databases
	// for now.
	dir, err := os.MkdirTemp("", "syncthing-db")
	if err != nil {
		return nil, wrap(err)
	}
	path := filepath.Join(dir, "db")
	l.Debugln("Test DB in", path)
	return Open(path)
}

func openCommon(sqlDB *sqlx.DB) (*DB, error) {
	if _, err := sqlDB.Exec(`PRAGMA auto_vacuum = INCREMENTAL`); err != nil {
		return nil, wrap(err, "PRAGMA auto_vacuum")
	}
	if _, err := sqlDB.Exec(`PRAGMA default_temp_store = MEMORY`); err != nil {
		return nil, wrap(err, "PRAGMA default_temp_store")
	}
	if _, err := sqlDB.Exec(`PRAGMA temp_store = MEMORY`); err != nil {
		return nil, wrap(err, "PRAGMA temp_store")
	}

	db := &DB{
		sql:        sqlDB,
		statements: make(map[string]*sqlx.Stmt),
	}

	if err := db.runScripts("sql/schema/*"); err != nil {
		return nil, wrap(err)
	}

	ver, _ := db.getAppliedSchemaVersion()
	if ver.SchemaVersion > 0 {
		filter := func(scr string) bool {
			scr = filepath.Base(scr)
			nstr, _, ok := strings.Cut(scr, "-")
			if !ok {
				return false
			}
			n, err := strconv.ParseInt(nstr, 10, 32)
			if err != nil {
				return false
			}
			return int(n) > ver.SchemaVersion
		}
		if err := db.runScripts("sql/migrations/*", filter); err != nil {
			return nil, wrap(err)
		}
	}

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	db.localDeviceIdx, _ = db.deviceIdxLocked(protocol.LocalDeviceID)

	// Set the current schema version, if not already set
	if err := db.setAppliedSchemaVersion(currentSchemaVersion); err != nil {
		return nil, wrap(err)
	}

	db.tplInput = map[string]any{
		"FlagLocalUnsupported": protocol.FlagLocalUnsupported,
		"FlagLocalIgnored":     protocol.FlagLocalIgnored,
		"FlagLocalMustRescan":  protocol.FlagLocalMustRescan,
		"FlagLocalReceiveOnly": protocol.FlagLocalReceiveOnly,
		"FlagLocalGlobal":      protocol.FlagLocalGlobal,
		"FlagLocalNeeded":      protocol.FlagLocalNeeded,
		"LocalDeviceIdx":       db.localDeviceIdx,
		"SyncthingVersion":     build.LongVersion,
	}

	return db, nil
}

var tplFuncs = template.FuncMap{
	"or": func(vs ...int) int {
		v := vs[0]
		for _, ov := range vs[1:] {
			v |= ov
		}
		return v
	},
}

// stmt returns a prepared statement for the given SQL string, after
// applying local template expansions. The statement is cached.
func (s *DB) stmt(tpl string) stmt {
	tpl = strings.TrimSpace(tpl)

	// Fast concurrent lookup of cached statement
	s.statementsMut.RLock()
	stmt, ok := s.statements[tpl]
	s.statementsMut.RUnlock()
	if ok {
		return stmt
	}

	// On miss, take the full lock, check again
	s.statementsMut.Lock()
	defer s.statementsMut.Unlock()
	stmt, ok = s.statements[tpl]
	if ok {
		return stmt
	}

	// Apply template expansions
	var sb strings.Builder
	compTpl := template.Must(template.New("tpl").Funcs(tplFuncs).Parse(tpl))
	if err := compTpl.Execute(&sb, s.tplInput); err != nil {
		panic("bug: bad template: " + err.Error())
	}

	// Prepare and cache
	stmt, err := s.sql.Preparex(sb.String())
	if err != nil {
		return failedStmt{err}
	}
	s.statements[tpl] = stmt
	return stmt
}

type stmt interface {
	Exec(args ...any) (sql.Result, error)
	Get(dest any, args ...any) error
	Queryx(args ...any) (*sqlx.Rows, error)
	Select(dest any, args ...any) error
}

type failedStmt struct {
	err error
}

func (f failedStmt) Exec(_ ...any) (sql.Result, error)   { return nil, f.err }
func (f failedStmt) Get(_ any, _ ...any) error           { return f.err }
func (f failedStmt) Queryx(_ ...any) (*sqlx.Rows, error) { return nil, f.err }
func (f failedStmt) Select(_ any, _ ...any) error        { return f.err }
