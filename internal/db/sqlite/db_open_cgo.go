//go:build cgo

package sqlite

import (
	_ "github.com/mattn/go-sqlite3" // register sqlite3 database driver
	"github.com/syncthing/syncthing/lib/build"
)

const (
	dbDriver      = "sqlite3"
	commonOptions = "_fk=true&_rt=true&_cslike=true&_cache_size=-65536&mode=rwc"
)

func init() {
	build.ExtraTags = append(build.ExtraTags, "c-sqlite")
}
