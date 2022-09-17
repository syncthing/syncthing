// Copyright (C) 2014-2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package assets hold utilities for serving static assets.
//
// The actual assets live in auto subpackages instead of here,
// because the set of assets varies per program.
package assets

import (
	"compress/gzip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// An Asset is an embedded file to be served over HTTP.
type Asset struct {
	Content  string // Contents of asset, possibly gzipped.
	Gzipped  bool
	Length   int       // Length of (decompressed) Content.
	Filename string    // Original filename, determines Content-Type.
	Modified time.Time // Determines ETag and Last-Modified.
}

// Serve writes a gzipped asset to w.
func Serve(w http.ResponseWriter, r *http.Request, asset Asset) {
	header := w.Header()

	mtype := MimeTypeForFile(asset.Filename)
	if mtype != "" {
		header.Set("Content-Type", mtype)
	}

	etag := fmt.Sprintf(`"%x"`, asset.Modified.Unix())
	header.Set("ETag", etag)
	header.Set("Last-Modified", asset.Modified.Format(http.TimeFormat))

	t, err := http.ParseTime(r.Header.Get("If-Modified-Since"))
	if err == nil && !asset.Modified.After(t) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	switch {
	case !asset.Gzipped:
		header.Set("Content-Length", strconv.Itoa(len(asset.Content)))
		io.WriteString(w, asset.Content)
	case strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"):
		header.Set("Content-Encoding", "gzip")
		header.Set("Content-Length", strconv.Itoa(len(asset.Content)))
		io.WriteString(w, asset.Content)
	default:
		header.Set("Content-Length", strconv.Itoa(asset.Length))
		// gunzip for browsers that don't want gzip.
		var gr *gzip.Reader
		gr, _ = gzip.NewReader(strings.NewReader(asset.Content))
		io.Copy(w, gr)
		gr.Close()
	}
}

// MimeTypeForFile returns the appropriate MIME type for an asset,
// based on the filename.
//
// We use a built in table of the common types since the system
// TypeByExtension might be unreliable. But if we don't know, we delegate
// to the system. All our text files are in UTF-8.
func MimeTypeForFile(file string) string {
	ext := filepath.Ext(file)
	switch ext {
	case ".htm", ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".eot":
		return "application/vnd.ms-fontobject"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml; charset=utf-8"
	case ".ttf":
		return "font/ttf"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return mime.TypeByExtension(ext)
	}
}
