package sqlite

import (
	"database/sql"
	"fmt"
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
		return nil, fmt.Errorf("open database: %w", err)
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
	if _, err := sqlDB.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return nil, wrap(err, "PRAGMA journal_mode")
	}
	if _, err := sqlDB.Exec(`PRAGMA auto_vacuum = INCREMENTAL`); err != nil {
		return nil, wrap(err, "PRAGMA auto_vacuum")
	}
	if _, err := sqlDB.Exec(`PRAGMA default_temp_store = MEMORY`); err != nil {
		return nil, wrap(err, "PRAGMA default_temp_store")
	}
	if _, err := sqlDB.Exec(`PRAGMA temp_store = MEMORY`); err != nil {
		return nil, wrap(err, "PRAGMA temp_store")
	}
	if _, err := sqlDB.Exec(`PRAGMA optimize = 0x10002`); err != nil {
		// https://www.sqlite.org/pragma.html#pragma_optimize
		return nil, wrap(err, "PRAGMA optimize")
	}
	if _, err := sqlDB.Exec(`PRAGMA journal_size_limit = 6144000;`); err != nil {
		// https://www.powersync.com/blog/sqlite-optimizations-for-ultra-high-performance
		return nil, wrap(err, "PRAGMA journal_size_limit")
	}

	sqlDB.SetMaxOpenConns(maxDBConns)

	db := &DB{
		sql:        sqlDB,
		statements: make(map[string]*sqlx.Stmt),
	}

	if err := db.runScripts("schema/*"); err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	ver, _ := db.getAppliedSchemaVersion()
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
	if err := db.runScripts("migrations/*", filter); err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	db.localDeviceIdx, _ = db.deviceIdxLocked(protocol.LocalDeviceID)

	// Set the initial schema version, if not already set
	if err := db.setAppliedSchemaVersion(currentSchemaVersion); err != nil {
		return nil, err
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
