// +build netbsd openbsd solaris

package du

import "errors"

var ErrUnsupported = errors.New("unsupported platform")

// Get returns the Usage of a given path, or an error if usage data is
// unavailable.
func Get(path string) (Usage, error) {
	return Usage{}, ErrUnsupported
}
