// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"cmp"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/protocol"
)

const currentSchemaVersion = 4

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

//nolint:noctx
func openBase(path string, maxConns int, pragmas, schemaScripts, migrationScripts []string) (*baseDB, error) {
	// Open the database with options to enable foreign keys and recursive
	// triggers (needed for the delete+insert triggers on row replace).
	pathURL := url.URL{
		Scheme:   "file",
		Path:     fileToUriPath(path),
		RawQuery: commonOptions,
	}
	sqlDB, err := sqlx.Open(dbDriver, pathURL.String())
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
	shouldVacuum := false
	if ver.SchemaVersion > 0 {
		type migration struct {
			script  string
			version int
		}
		migrations := make([]migration, 0, len(migrationScripts))
		for _, script := range migrationScripts {
			base := filepath.Base(script)
			nstr, _, ok := strings.Cut(base, "-")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(nstr, 10, 32)
			if err != nil {
				continue
			}
			migrations = append(migrations, migration{
				script:  script,
				version: int(n),
			})
		}
		slices.SortFunc(migrations, func(m1, m2 migration) int { return cmp.Compare(m1.version, m2.version) })
		for _, m := range migrations {
			if err := db.applyMigration(m.version, m.script); err != nil {
				return nil, wrap(err)
			}
			shouldVacuum = true
		}
	}

	// Set the current schema version, if not already set
	if err := setAppliedSchemaVersion(currentSchemaVersion, db.sql); err != nil {
		return nil, wrap(err)
	}

	if shouldVacuum {
		// We applied migrations and should take the opportunity to vaccuum
		// the database.
		if err := db.vacuumAndOptimize(); err != nil {
			return nil, wrap(err)
		}
	}

	return db, nil
}

func fileToUriPath(path string) string {
	path = filepath.ToSlash(path)
	if (build.IsWindows && len(path) >= 2 && path[1] == ':') ||
		(strings.HasPrefix(path, "//") && !strings.HasPrefix(path, "///")) {
		// Add an extra leading slash for Windows drive letter or UNC path
		path = "/" + path
	}
	return path
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

func (s *baseDB) vacuumAndOptimize() error {
	stmts := []string{
		"VACUUM;",
		"PRAGMA optimize;",
		"PRAGMA wal_checkpoint(truncate);",
	}
	for _, stmt := range stmts {
		if _, err := s.sql.Exec(stmt); err != nil {
			return wrap(err, stmt)
		}
	}
	return nil
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

//nolint:noctx
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
		if err := s.execScript(tx, scr); err != nil {
			return wrap(err)
		}
	}

	return wrap(tx.Commit())
}

//nolint:noctx
func (s *baseDB) applyMigration(ver int, script string) error {
	tx, err := s.sql.Begin()
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck

	slog.Info("Applying database migration", slogutil.FilePath(s.baseName), "toSchema", ver, "script", script)

	if err := s.execScript(tx, script); err != nil {
		return wrap(err)
	}

	if err := setAppliedSchemaVersion(ver, tx); err != nil {
		return wrap(err)
	}

	return wrap(tx.Commit())
}

//nolint:noctx
func (s *baseDB) execScript(tx *sql.Tx, scr string) error {
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
	return nil
}

type schemaVersion struct {
	SchemaVersion    int
	AppliedAt        int64
	SyncthingVersion string
}

func (s *schemaVersion) AppliedTime() time.Time {
	return time.Unix(0, s.AppliedAt)
}

func setAppliedSchemaVersion(ver int, execer sqlx.Execer) error {
	_, err := execer.Exec(`
		INSERT OR IGNORE INTO schemamigrations (schema_version, applied_at, syncthing_version)
		VALUES (?, ?, ?)
	`, ver, time.Now().UnixNano(), build.LongVersion)
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
