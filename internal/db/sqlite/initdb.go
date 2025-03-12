package sqlite

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed init/*.sql
var initScripts embed.FS

func initDB(db *sqlx.DB) error {
	initFs, err := fs.Sub(initScripts, "init")
	if err != nil {
		return wrap("init", err)
	}
	scripts, err := fs.ReadDir(initFs, ".")
	if err != nil {
		return wrap("init", err)
	}
	for _, scr := range scripts {
		bs, err := fs.ReadFile(initFs, scr.Name())
		if err != nil {
			return wrap("init", err)
		}
		// SQLite requires one statement per exec, so we split the init
		// files on lines containing only a semicolon and execute them
		// separately. We require it on a separate line because there are
		// also statement-internal semicolons in the triggers.
		for _, stmt := range strings.Split(string(bs), "\n;") {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("init statement: %s: %w", stmt, err)
			}
		}
	}

	return nil
}
