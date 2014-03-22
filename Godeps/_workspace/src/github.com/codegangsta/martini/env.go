package martini

import (
	"os"
)

const (
	Dev  string = "development"
	Prod string = "production"
	Test string = "test"
)

// Env is the environment that Martini is executing in. The MARTINI_ENV is read on initialization to set this variable.
var Env string = Dev

func init() {
	e := os.Getenv("MARTINI_ENV")
	if len(e) > 0 {
		Env = e
	}
}
