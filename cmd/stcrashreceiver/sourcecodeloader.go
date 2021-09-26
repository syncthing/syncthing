// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	urlPrefix   = "https://raw.githubusercontent.com/syncthing/syncthing/"
	httpTimeout = 10 * time.Second
)

type githubSourceCodeLoader struct {
	mut     sync.Mutex
	version string
	cache   map[string]map[string][][]byte // version -> file -> lines
	client  *http.Client
}

func newGithubSourceCodeLoader() *githubSourceCodeLoader {
	return &githubSourceCodeLoader{
		cache:  make(map[string]map[string][][]byte),
		client: &http.Client{Timeout: httpTimeout},
	}
}

func (l *githubSourceCodeLoader) LockWithVersion(version string) {
	l.mut.Lock()
	l.version = version
	if _, ok := l.cache[version]; !ok {
		l.cache[version] = make(map[string][][]byte)
	}
}

func (l *githubSourceCodeLoader) Unlock() {
	l.mut.Unlock()
}

func (l *githubSourceCodeLoader) Load(filename string, line, context int) ([][]byte, int) {
	filename = filepath.ToSlash(filename)
	lines, ok := l.cache[l.version][filename]
	if !ok {
		// Cache whatever we managed to find (or nil if nothing, so we don't try again)
		defer func() {
			l.cache[l.version][filename] = lines
		}()

		knownPrefixes := []string{"/lib/", "/cmd/"}
		var idx int
		for _, pref := range knownPrefixes {
			idx = strings.Index(filename, pref)
			if idx >= 0 {
				break
			}
		}
		if idx == -1 {
			return nil, 0
		}

		url := urlPrefix + l.version + filename[idx:]
		resp, err := l.client.Get(url)

		if err != nil {
			fmt.Println("Loading source:", err)
			return nil, 0
		}
		if resp.StatusCode != http.StatusOK {
			fmt.Println("Loading source:", resp.Status)
			return nil, 0
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			fmt.Println("Loading source:", err.Error())
			return nil, 0
		}
		lines = bytes.Split(data, []byte{'\n'})
	}

	return getLineFromLines(lines, line, context)
}

func getLineFromLines(lines [][]byte, line, context int) ([][]byte, int) {
	if lines == nil {
		// cached error from ReadFile: return no lines
		return nil, 0
	}

	line-- // stack trace lines are 1-indexed
	start := line - context
	var idx int
	if start < 0 {
		start = 0
		idx = line
	} else {
		idx = context
	}
	end := line + context + 1
	if line >= len(lines) {
		return nil, 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	return lines[start:end], idx
}
