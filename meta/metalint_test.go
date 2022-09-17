// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package meta

import (
	"bytes"
	"log"
	"os/exec"
	"strings"
	"testing"
)

var (
	// fast linters complete in a fraction of a second and might as well be
	// run always as part of the build
	fastLinters = []string{
		"deadcode",
		"golint",
		"ineffassign",
		"vet",
	}

	// slow linters take several seconds and are run only as part of the
	// "metalint" command.
	slowLinters = []string{
		"gosimple",
		"staticcheck",
		"structcheck",
		"unused",
		"varcheck",
	}

	// Which parts of the tree to lint
	lintDirs = []string{
		".",
		"../cmd/...",
		"../lib/...",
		"../script/...",
	}

	// Messages to ignore
	lintExcludes = []string{
		".pb.go",
		"should have comment",
		"protocol.Vector composite literal uses unkeyed fields",
		"cli.Requires composite literal uses unkeyed fields",
		"Use DialContext instead",   // Go 1.7
		"os.SEEK_SET is deprecated", // Go 1.7
		"SA4017",                    // staticcheck "is a pure function but its return value is ignored"
	}
)

func TestCheckMetalint(t *testing.T) {
	if !isGometalinterInstalled() {
		return
	}

	gometalinter(t, lintDirs, lintExcludes...)
}

func isGometalinterInstalled() bool {
	if _, err := runError("gometalinter", "--disable-all"); err != nil {
		log.Println("gometalinter is not installed")
		return false
	}
	return true
}

func gometalinter(_ *testing.T, dirs []string, excludes ...string) bool {
	params := []string{"--disable-all", "--concurrency=2", "--deadline=300s"}

	for _, linter := range fastLinters {
		params = append(params, "--enable="+linter)
	}

	if !testing.Short() {
		for _, linter := range slowLinters {
			params = append(params, "--enable="+linter)
		}
	}

	for _, exclude := range excludes {
		params = append(params, "--exclude="+exclude)
	}

	params = append(params, dirs...)

	bs, _ := runError("gometalinter", params...)

	nerr := 0
	lines := make(map[string]struct{})
	for _, line := range strings.Split(string(bs), "\n") {
		if line == "" {
			continue
		}
		if _, ok := lines[line]; ok {
			continue
		}
		log.Println(line)
		if strings.Contains(line, "executable file not found") {
			log.Println(` - Try "go run build.go setup" to install missing tools`)
		}
		lines[line] = struct{}{}
		nerr++
	}

	return nerr == 0
}

func runError(cmd string, args ...string) ([]byte, error) {
	ecmd := exec.Command(cmd, args...)
	bs, err := ecmd.CombinedOutput()
	return bytes.TrimSpace(bs), err
}
