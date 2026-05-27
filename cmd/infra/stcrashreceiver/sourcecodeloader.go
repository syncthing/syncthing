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

	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	urlPrefix       = "https://raw.githubusercontent.com/syncthing/syncthing/"
	httpTimeout     = 10 * time.Second
	maxCacheEntries = 1000
)

type cacheKey struct {
	version string
	file    string
}

type githubSourceCodeLoader struct {
	mut     sync.Mutex
	version string

	cache  *lru.TwoQueueCache[cacheKey, [][]byte] // version & file -> lines
	client *http.Client
}

func newGithubSourceCodeLoader() *githubSourceCodeLoader {
	cache, _ := lru.New2Q[cacheKey, [][]byte](maxCacheEntries)
	return &githubSourceCodeLoader{
		cache:  cache,
		client: &http.Client{Timeout: httpTimeout},
	}
}

func (l *githubSourceCodeLoader) LockWithVersion(version string) {
	l.mut.Lock()
	l.version = version
}

func (l *githubSourceCodeLoader) Unlock() {
	l.mut.Unlock()
}

func (l *githubSourceCodeLoader) Load(filename string, line, context int) ([][]byte, int) {
	filename = filepath.ToSlash(filename)
	key := cacheKey{version: l.version, file: filename}
	lines, ok := l.cache.Get(key)
	if !ok {
		// Cache whatever we managed to find (or nil if nothing, so we don't try again)
		defer func() {
			l.cache.Add(key, lines)
			metricSourceCodeCacheSize.Set(float64(l.cache.Len()))
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
			metricSourceCodeLoadsTotal.WithLabelValues("failed").Inc()
			return nil, 0
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Println("Loading source:", resp.Status)
			metricSourceCodeLoadsTotal.WithLabelValues("failed").Inc()
			return nil, 0
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Loading source:", err.Error())
			metricSourceCodeLoadsTotal.WithLabelValues("failed").Inc()
			return nil, 0
		}
		lines = bytes.Split(data, []byte{'\n'})
		metricSourceCodeLoadsTotal.WithLabelValues("loaded").Inc()
	} else {
		metricSourceCodeLoadsTotal.WithLabelValues("cached").Inc()
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
