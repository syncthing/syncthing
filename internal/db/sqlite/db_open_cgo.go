//go:build cgo

package sqlite

import _ "github.com/mattn/go-sqlite3" // register sqlite3 database driver

const (
	dbDriver      = "sqlite3"
	commonOptions = "_fk=true&_rt=true&mode=rwc"
)
