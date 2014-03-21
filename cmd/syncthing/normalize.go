//+build !darwin

package main

import "code.google.com/p/go.text/unicode/norm"

// FSNormalize returns the string with the required unicode normalization for
// the host operating system.
func FSNormalize(s string) string {
	return norm.NFC.String(s)
}
