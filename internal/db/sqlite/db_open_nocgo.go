//go:build !cgo

package sqlite

import _ "modernc.org/sqlite" // register sqlite database driver

const (
	dbDriver      = "sqlite"
	commonOptions = "_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)&mode=rwc"
)
