// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/auto"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/sync"
)

type staticsServer struct {
	assetDir        string
	assets          map[string][]byte
	availableThemes []string

	mut   sync.RWMutex
	theme string
}

func newStaticsServer(theme, assetDir string) *staticsServer {
	s := &staticsServer{
		assetDir: assetDir,
		assets:   auto.Assets(),
		mut:      sync.NewRWMutex(),
		theme:    theme,
	}

	seen := make(map[string]struct{})
	// Load themes from compiled in assets.
	for file := range auto.Assets() {
		theme := strings.Split(file, "/")[0]
		if _, ok := seen[theme]; !ok {
			seen[theme] = struct{}{}
			s.availableThemes = append(s.availableThemes, theme)
		}
	}
	if assetDir != "" {
		// Load any extra themes from the asset override dir.
		for _, dir := range dirNames(assetDir) {
			if _, ok := seen[dir]; !ok {
				seen[dir] = struct{}{}
				s.availableThemes = append(s.availableThemes, dir)
			}
		}
	}

	return s
}

func (s *staticsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/themes.json":
		s.serveThemes(w, r)
	default:
		s.serveAsset(w, r)
	}
}

func (s *staticsServer) serveAsset(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Path

	if file[0] == '/' {
		file = file[1:]
	}

	if len(file) == 0 {
		file = "index.html"
	}

	s.mut.RLock()
	theme := s.theme
	s.mut.RUnlock()

	// Check for an override for the current theme.
	if s.assetDir != "" {
		p := filepath.Join(s.assetDir, theme, filepath.FromSlash(file))
		if _, err := os.Stat(p); err == nil {
			http.ServeFile(w, r, p)
			return
		}
	}

	// Check for a compiled in asset for the current theme.
	bs, ok := s.assets[theme+"/"+file]
	if !ok {
		// Check for an overridden default asset.
		if s.assetDir != "" {
			p := filepath.Join(s.assetDir, config.DefaultTheme, filepath.FromSlash(file))
			if _, err := os.Stat(p); err == nil {
				http.ServeFile(w, r, p)
				return
			}
		}

		// Check for a compiled in default asset.
		bs, ok = s.assets[config.DefaultTheme+"/"+file]
		if !ok {
			http.NotFound(w, r)
			return
		}
	}

	mtype := s.mimeTypeForFile(file)
	if len(mtype) != 0 {
		w.Header().Set("Content-Type", mtype)
	}
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
	} else {
		// ungzip if browser not send gzip accepted header
		var gr *gzip.Reader
		gr, _ = gzip.NewReader(bytes.NewReader(bs))
		bs, _ = ioutil.ReadAll(gr)
		gr.Close()
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))

	w.Write(bs)
}

func (s *staticsServer) serveThemes(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string][]string{
		"themes": s.availableThemes,
	})
}

func (s *staticsServer) mimeTypeForFile(file string) string {
	// We use a built in table of the common types since the system
	// TypeByExtension might be unreliable. But if we don't know, we delegate
	// to the system. All our files are UTF-8.
	ext := filepath.Ext(file)
	switch ext {
	case ".htm", ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".ttf":
		return "application/x-font-ttf"
	case ".woff":
		return "application/x-font-woff"
	case ".svg":
		return "image/svg+xml; charset=utf-8"
	default:
		return mime.TypeByExtension(ext)
	}
}

func (s *staticsServer) setTheme(theme string) {
	s.mut.Lock()
	s.theme = theme
	s.mut.Unlock()
}

func (s *staticsServer) String() string {
	return fmt.Sprintf("staticsServer@%p", s)
}
