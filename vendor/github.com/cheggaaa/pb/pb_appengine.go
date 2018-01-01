// +build appengine

package pb

import "errors"

// terminalWidth returns width of the terminal, which is not supported
// and should always failed on appengine classic which is a sandboxed PaaS.
func terminalWidth() (int, error) {
	return 0, errors.New("Not supported")
}
