// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestGithubSourceCodeLoaderClosesBodyOnHTTPError(t *testing.T) {
	loader := newGithubSourceCodeLoader()
	body := &trackingReadCloser{Reader: strings.NewReader("not found")}
	loader.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Status:     "404 Not Found",
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}

	loader.LockWithVersion("main")
	defer loader.Unlock()

	lines, idx := loader.Load("/tmp/build/lib/model/model.go", 1, 0)
	if lines != nil || idx != 0 {
		t.Fatalf("unexpected result idx=%d lines=%q", idx, lines)
	}
	if !body.closed {
		t.Fatal("expected response body to be closed")
	}
}
