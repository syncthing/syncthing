// +build windows

package main

import "os/exec"

func openURL(url string) error {
	return exec.Command("cmd.exe", "/C", "start "+url).Run()
}
