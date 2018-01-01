package altsrc

import (
	"time"

	"gopkg.in/urfave/cli.v1"
)

// InputSourceContext is an interface used to allow
// other input sources to be implemented as needed.
type InputSourceContext interface {
	Int(name string) (int, error)
	Duration(name string) (time.Duration, error)
	Float64(name string) (float64, error)
	String(name string) (string, error)
	StringSlice(name string) ([]string, error)
	IntSlice(name string) ([]int, error)
	Generic(name string) (cli.Generic, error)
	Bool(name string) (bool, error)
	BoolT(name string) (bool, error)
}
