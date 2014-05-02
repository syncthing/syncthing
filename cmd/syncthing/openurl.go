/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os/exec"
	"runtime"
	"syscall"
)

func openURL(url string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/C", "start "+url).Run()
	}

	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Run()
	}

	cmd := exec.Command("xdg-open", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return cmd.Run()
}
