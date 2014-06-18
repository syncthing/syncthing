// +build solaris

package main

import (
	"os/exec"
	"strconv"
)

func memorySize() (uint64, error) {
	cmd := exec.Command("prtconf", "-m")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	mb, err := strconv.ParseUint(string(out), 10, 64)
	if err != nil {
		return 0, err
	}
	return mb * 1024 * 1024, nil
}
