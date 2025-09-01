//go:build !windows
// +build !windows

package main

func InitConsole() error { return nil }
func FreeConsole()       {}
