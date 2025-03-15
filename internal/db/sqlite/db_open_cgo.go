//go:build cgo

package sqlite

import (
	_ "github.com/mattn/go-sqlite3" // register sqlite3 database driver
	"github.com/syncthing/syncthing/lib/build"
)

const (
	dbDriver      = "sqlite3"
	commonOptions = "_fk=true&_rt=true&_cache_size=-65536&_sync=1"
)

func init() {
	build.AddTag("mattn-sqlite")
}
