package main

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func memorySize() (uint64, error) {
	cmd := exec.Command("sysctl", "hw.memsize")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	fs := strings.Fields(string(out))
	if len(fs) != 2 {
		return 0, errors.New("sysctl parse error")
	}
	bytes, err := strconv.ParseUint(fs[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return bytes, nil
}
