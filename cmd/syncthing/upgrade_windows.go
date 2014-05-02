// +build windows

package main

import "errors"

func upgrade() error {
	return errors.New("Upgrade currently unsupported on Windows")
}
