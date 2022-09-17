// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package assets

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func compress(s string) string {
	var sb strings.Builder
	gz := gzip.NewWriter(&sb)

	io.WriteString(gz, s)
	gz.Close()
	return sb.String()
}

func decompress(p []byte) (out []byte) {
	r, err := gzip.NewReader(bytes.NewBuffer(p))
	if err == nil {
		out, err = io.ReadAll(r)
	}
	if err != nil {
		panic(err)
	}
	return out
}

func TestServe(t *testing.T)     { testServe(t, false) }
func TestServeGzip(t *testing.T) { testServe(t, true) }

func testServe(t *testing.T, gzip bool) {
	const indexHTML = `<html>Hello, world!</html>`
	content := indexHTML
	if gzip {
		content = compress(indexHTML)
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		Serve(w, r, Asset{
			Content:  content,
			Gzipped:  gzip,
			Length:   len(indexHTML),
			Filename: r.URL.Path[1:],
			Modified: time.Unix(0, 0),
		})
	}

	for _, acceptGzip := range []bool{true, false} {
		r := httptest.NewRequest("GET", "http://localhost/index.html", nil)
		if acceptGzip {
			r.Header.Set("accept-encoding", "gzip, deflate")
		}

		w := httptest.NewRecorder()
		handler(w, r)
		res := w.Result()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("wanted OK, got status %d", res.StatusCode)
		}
		if ctype := res.Header.Get("Content-Type"); ctype != "text/html; charset=utf-8" {
			t.Errorf("unexpected Content-Type %q", ctype)
		}
		// ETags must be quoted ASCII strings:
		// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/ETag
		if etag := res.Header.Get("ETag"); etag != `"0"` {
			t.Errorf("unexpected ETag %q", etag)
		}

		body, _ := io.ReadAll(res.Body)

		// Content-Length is the number of bytes in the encoded (compressed) body
		// (https://stackoverflow.com/a/3819303).
		n, err := strconv.Atoi(res.Header.Get("Content-Length"))
		if err != nil {
			t.Errorf("malformed Content-Length %q", res.Header.Get("Content-Length"))
		} else if n != len(body) {
			t.Errorf("wrong Content-Length %d, should be %d", n, len(body))
		}

		if gzip && acceptGzip {
			body = decompress(body)
		}
		if string(body) != indexHTML {
			t.Fatalf("unexpected content %q", body)
		}
	}

	r := httptest.NewRequest("GET", "http://localhost/index.html", nil)
	r.Header.Set("if-none-match", `"0"`)
	w := httptest.NewRecorder()
	handler(w, r)
	res := w.Result()

	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("wanted NotModified, got status %d", res.StatusCode)
	}

	r = httptest.NewRequest("GET", "http://localhost/index.html", nil)
	r.Header.Set("if-modified-since", time.Now().Format(http.TimeFormat))
	w = httptest.NewRecorder()
	handler(w, r)
	res = w.Result()

	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("wanted NotModified, got status %d", res.StatusCode)
	}
}
