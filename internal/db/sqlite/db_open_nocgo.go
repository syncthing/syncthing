//go:build !cgo

package sqlite

import (
	"github.com/syncthing/syncthing/lib/build"
	_ "modernc.org/sqlite"
) // register sqlite database driver

const (
	dbDriver      = "sqlite"
	commonOptions = "_pragma=foreign_keys(1)&_pragma=recursive_triggers(1)&_pragma=cache_size(-65536)&_pragma=case_sensitive_like(1)&mode=rwc"
)

func init() {
	build.AddTag("go-sqlite")
}
