package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

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
	// Set up initial tables, indexes, triggers.
	if err := initDB(sqlDB); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB.SetMaxOpenConns(128)

	db := &DB{
		sql:        sqlDB,
		statements: make(map[string]*sqlx.Stmt),
	}

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	db.localDeviceIdx, _ = db.deviceIdxLocked(protocol.LocalDeviceID)

	db.tplInput = map[string]any{
		"FlagLocalUnsupported": protocol.FlagLocalUnsupported,
		"FlagLocalIgnored":     protocol.FlagLocalIgnored,
		"FlagLocalMustRescan":  protocol.FlagLocalMustRescan,
		"FlagLocalReceiveOnly": protocol.FlagLocalReceiveOnly,
		"FlagLocalGlobal":      protocol.FlagLocalGlobal,
		"FlagLocalNeeded":      protocol.FlagLocalNeeded,
		"FlagLocalDeleted":     protocol.FlagLocalDeleted,
		"LocalDeviceIdx":       db.localDeviceIdx,
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

func (s *DB) tpl(tpl string) stmt {
	s.statementsMut.RLock()
	stmt, ok := s.statements[tpl]
	s.statementsMut.RUnlock()
	if ok {
		return stmt
	}

	s.statementsMut.Lock()
	defer s.statementsMut.Unlock()
	stmt, ok = s.statements[tpl]
	if ok {
		return stmt
	}

	var sb strings.Builder
	compTpl := template.Must(template.New("tpl").Funcs(tplFuncs).Parse(tpl))
	if err := compTpl.Execute(&sb, s.tplInput); err != nil {
		panic("bug: bad template: " + err.Error())
	}
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
