// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bufio"
	"bytes"
	"errors"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/maruel/panicparse/stack"
)

// syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC
var longVersionRE = regexp.MustCompile(`syncthing\s+(v[^\s]+)\s+"([^"]+)"\s\(([^\s]+)\s+([^-]+)-([^)]+)\)\s+([^\s]+)`)

type version struct {
	version string // "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep"
	tag     string // "v1.1.4-rc.1"
	commit  string // "6aaae618", blank when absent
	code    string // "Erbium Earthworm"
	runtime string // "go1.12.5"
	goos    string // "darwin"
	goarch  string // "amd64"
	builder string // "jb@kvin.kastelo.net"
}

func parseVersion(line string) (version, error) {
	m := longVersionRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return version{}, errors.New("unintelligeble version string")
	}

	v := version{
		version: m[1],
		code:    m[2],
		runtime: m[3],
		goos:    m[4],
		goarch:  m[5],
		builder: m[6],
	}
	parts := strings.Split(v.version, "+")
	v.tag = parts[0]
	if len(parts) > 1 {
		fields := strings.Split(parts[1], "-")
		if len(fields) >= 2 && strings.HasPrefix(fields[1], "g") {
			v.commit = fields[1][1:]
		}
	}

	return v, nil
}

func reportToSentry(report []byte) error {
	r := bytes.NewReader(report)
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		return errors.New("no first line")
	}

	version, err := parseVersion(sc.Text())
	if err != nil {
		return err
	}

	ctx, err := stack.ParseDump(r, ioutil.Discard, false)
	if err != nil {
		return err
	}

	return nil
}
