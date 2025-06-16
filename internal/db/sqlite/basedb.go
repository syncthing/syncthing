// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"database/sql"
	"embed"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
)

const currentSchemaVersion = 2

//go:embed sql/**
var embedded embed.FS

type baseDB struct {
	path     string
	baseName string
	sql      *sqlx.DB

	updateLock       sync.Mutex
	updatePoints     int
	checkpointsCount int

	statementsMut sync.RWMutex
	statements    map[string]*sqlx.Stmt
	tplInput      map[string]any
}

func openBase(path string, maxConns int, pragmas, schemaScripts, migrationScripts []string) (*baseDB, error) {
	// Open the database with options to enable foreign keys and recursive
	// triggers (needed for the delete+insert triggers on row replace).
	sqlDB, err := sqlx.Open(dbDriver, "file:"+path+"?"+commonOptions)
	if err != nil {
		return nil, wrap(err)
	}

	sqlDB.SetMaxOpenConns(maxConns)

	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec("PRAGMA " + pragma); err != nil {
			return nil, wrap(err, "PRAGMA "+pragma)
		}
	}

	db := &baseDB{
		path:       path,
		baseName:   filepath.Base(path),
		sql:        sqlDB,
		statements: make(map[string]*sqlx.Stmt),
		tplInput: map[string]any{
			"FlagLocalUnsupported":   protocol.FlagLocalUnsupported,
			"FlagLocalIgnored":       protocol.FlagLocalIgnored,
			"FlagLocalMustRescan":    protocol.FlagLocalMustRescan,
			"FlagLocalReceiveOnly":   protocol.FlagLocalReceiveOnly,
			"FlagLocalGlobal":        protocol.FlagLocalGlobal,
			"FlagLocalNeeded":        protocol.FlagLocalNeeded,
			"FlagLocalRemoteInvalid": protocol.FlagLocalRemoteInvalid,
			"LocalInvalidFlags":      protocol.LocalInvalidFlags,
			"SyncthingVersion":       build.LongVersion,
		},
	}

	for _, script := range schemaScripts {
		if err := db.runScripts(script); err != nil {
			return nil, wrap(err)
		}
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
		for _, script := range migrationScripts {
			if err := db.runScripts(script, filter); err != nil {
				return nil, wrap(err)
			}
		}
	}

	// Set the current schema version, if not already set
	if err := db.setAppliedSchemaVersion(currentSchemaVersion); err != nil {
		return nil, wrap(err)
	}

	return db, nil
}

func (s *baseDB) Close() error {
	s.updateLock.Lock()
	s.statementsMut.Lock()
	defer s.updateLock.Unlock()
	defer s.statementsMut.Unlock()
	for _, stmt := range s.statements {
		stmt.Close()
	}
	return wrap(s.sql.Close())
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
func (s *baseDB) stmt(tpl string) stmt {
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

	// Prepare and cache
	stmt, err := s.sql.Preparex(s.expandTemplateVars(tpl))
	if err != nil {
		return failedStmt{err}
	}
	s.statements[tpl] = stmt
	return stmt
}

// expandTemplateVars just applies template expansions to the template
// string, or dies trying
func (s *baseDB) expandTemplateVars(tpl string) string {
	var sb strings.Builder
	compTpl := template.Must(template.New("tpl").Funcs(tplFuncs).Parse(tpl))
	if err := compTpl.Execute(&sb, s.tplInput); err != nil {
		panic("bug: bad template: " + err.Error())
	}
	return sb.String()
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

func (s *baseDB) runScripts(glob string, filter ...func(s string) bool) error {
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
				continue nextScript
			}
		}
		bs, err := fs.ReadFile(embedded, scr)
		if err != nil {
			return wrap(err, scr)
		}
		// SQLite requires one statement per exec, so we split the init
		// files on lines containing only a semicolon and execute them
		// separately. We require it on a separate line because there are
		// also statement-internal semicolons in the triggers.
		for _, stmt := range strings.Split(string(bs), "\n;") {
			if _, err := tx.Exec(s.expandTemplateVars(stmt)); err != nil {
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

func (s *baseDB) setAppliedSchemaVersion(ver int) error {
	_, err := s.stmt(`
		INSERT OR IGNORE INTO schemamigrations (schema_version, applied_at, syncthing_version)
		VALUES (?, ?, ?)
	`).Exec(ver, time.Now().UnixNano(), build.LongVersion)
	return wrap(err)
}

func (s *baseDB) getAppliedSchemaVersion() (schemaVersion, error) {
	var v schemaVersion
	err := s.stmt(`
		SELECT schema_version as schemaversion, applied_at as appliedat, syncthing_version as syncthingversion FROM schemamigrations
		ORDER BY schema_version DESC
		LIMIT 1
	`).Get(&v)
	return v, wrap(err)
}
