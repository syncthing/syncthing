// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGithubSourceCodeLoaderLoadsInternalPaths(t *testing.T) {
	loader := newGithubSourceCodeLoader()
	var gotPath string
	loader.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotPath = req.URL.Path
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("first\nsecond\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	loader.LockWithVersion("main")
	defer loader.Unlock()

	lines, idx := loader.Load("/tmp/build/internal/db/sqlite/util.go", 2, 0)
	if gotPath != "/syncthing/syncthing/main/internal/db/sqlite/util.go" {
		t.Fatalf("unexpected request path %q", gotPath)
	}
	if idx != 0 || len(lines) != 1 || string(lines[0]) != "second" {
		t.Fatalf("unexpected result idx=%d lines=%q", idx, lines)
	}
}

func TestGithubSourceCodeLoaderSkipsUnknownPaths(t *testing.T) {
	loader := newGithubSourceCodeLoader()
	called := false
	loader.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		}),
	}

	loader.LockWithVersion("main")
	defer loader.Unlock()

	lines, idx := loader.Load("/tmp/build/pkg/unrelated/file.go", 1, 0)
	if called {
		t.Fatal("unexpected fetch for unknown path")
	}
	if lines != nil || idx != 0 {
		t.Fatalf("unexpected result idx=%d lines=%q", idx, lines)
	}
}
