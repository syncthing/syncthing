package main

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func memorySize() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	s := bufio.NewScanner(f)
	if !s.Scan() {
		return 0, errors.New("/proc/meminfo parse error 1")
	}

	l := s.Text()
	fs := strings.Fields(l)
	if len(fs) != 3 || fs[2] != "kB" {
		return 0, errors.New("/proc/meminfo parse error 2")
	}

	kb, err := strconv.ParseUint(fs[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return kb * 1024, nil
}
