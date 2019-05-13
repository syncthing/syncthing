// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sentry

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/getsentry/raven-go"
	"github.com/syncthing/syncthing/lib/build"
)

const (
	packagePrefix = "github.com/syncthing/syncthing"
	urlPrefix     = "https://raw.githubusercontent.com/syncthing/syncthing/"
)

type chainedSourceCodeLoader struct {
	loaders []raven.SourceCodeLoader
}

func (l *chainedSourceCodeLoader) Load(filename string, line, context int) ([][]byte, int) {
	for _, loader := range l.loaders {
		data, n := loader.Load(filename, line, context)
		if data != nil {
			return data, n
		}
	}
	return nil, 0
}

type githubSourceCodeLoader struct {
	mut   sync.Mutex
	cache map[string][][]byte
}

func (l *githubSourceCodeLoader) Load(filename string, line, context int) ([][]byte, int) {
	lines, ok := l.cache[filename]
	if !ok {
		// Cache whatever we managed to find (or nil if nothing, so we don't try again)
		defer func() {
			l.cache[filename] = lines
		}()

		idx := strings.Index(filename, packagePrefix)
		if idx == -1 {
			return nil, 0
		}

		url := urlPrefix + build.Version + filename[idx+len(packagePrefix):]
		resp, err := http.Get(url)

		if err != nil || resp.StatusCode != http.StatusOK {
			return nil, 0
		}
		data, err := ioutil.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			fmt.Println(err.Error())
			return nil, 0
		}
		lines = bytes.Split(data, []byte{'\n'})
	}

	return getLineFromLines(lines, line, context)
}

// Copy paste from raven package, because it's unexported.
type fsLoader struct {
	mu    sync.Mutex
	cache map[string][][]byte
}

func (fs *fsLoader) Load(filename string, line, context int) ([][]byte, int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	lines, ok := fs.cache[filename]
	if !ok {
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			// cache errors as nil slice: code below handles it correctly
			// otherwise when missing the source or running as a different user, we try
			// reading the file on each error which is unnecessary
			fs.cache[filename] = nil
			return nil, 0
		}
		lines = bytes.Split(data, []byte{'\n'})
		fs.cache[filename] = lines
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
