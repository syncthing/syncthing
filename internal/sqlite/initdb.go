package sqlite

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

//go:embed init/*.sql
var initScripts embed.FS

var tplParams = map[string]any{
	"FileInfoTypes": []int64{
		int64(protocol.FileInfoTypeFile),
		int64(protocol.FileInfoTypeDirectory),
		int64(protocol.FileInfoTypeSymlink),
	},
	"LocalFlagBits": []int64{
		0, // no flags set
		protocol.FlagLocalUnsupported,
		protocol.FlagLocalIgnored,
		protocol.FlagLocalMustRescan,
		protocol.FlagLocalReceiveOnly,
		flagNeed,
	},
}

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
		tpl := template.Must(template.New(scr.Name()).Parse(string(bs)))
		buf := new(bytes.Buffer)
		if err := tpl.Execute(buf, tplParams); err != nil {
			return wrap("init", err)
		}
		for _, stmt := range strings.Split(buf.String(), "\n;") {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("init statement: %s: %w", stmt, err)
			}
		}
	}
	return nil
}
