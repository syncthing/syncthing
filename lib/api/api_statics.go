// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/api/auto"
	"github.com/syncthing/syncthing/lib/assets"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/sync"
)

const themePrefix = "theme-assets/"

type staticsServer struct {
	assetDir        string
	assets          map[string]assets.Asset
	availableThemes []string

	mut             sync.RWMutex
	theme           string
	lastThemeChange time.Time
}

func newStaticsServer(theme, assetDir string) *staticsServer {
	s := &staticsServer{
		assetDir:        assetDir,
		assets:          auto.Assets(),
		mut:             sync.NewRWMutex(),
		theme:           theme,
		lastThemeChange: time.Now().UTC(),
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
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	file := r.URL.Path

	if file[0] == '/' {
		file = file[1:]
	}

	if len(file) == 0 {
		file = "index.html"
	}

	s.mut.RLock()
	theme := s.theme
	modificationTime := s.lastThemeChange
	s.mut.RUnlock()

	// If path starts with special prefix, get theme and file from path
	if strings.HasPrefix(file, themePrefix) {
		path := file[len(themePrefix):]
		i := strings.IndexRune(path, '/')

		if i == -1 {
			http.NotFound(w, r)
			return
		}

		theme = path[:i]
		file = path[i+1:]
	}

	// Check for an override for the current theme.
	if s.serveFromAssetDir(file, theme, w, r) {
		return
	}

	// Check for a compiled in asset for the current theme.
	if s.serveFromAssets(file, theme, modificationTime, w, r) {
		return
	}

	// Check for an overridden default asset.
	if s.serveFromAssetDir(file, config.DefaultTheme, w, r) {
		return
	}

	// Check for a compiled in default asset.
	if s.serveFromAssets(file, config.DefaultTheme, modificationTime, w, r) {
		return
	}

	http.NotFound(w, r)
}

func (s *staticsServer) serveFromAssetDir(file, theme string, w http.ResponseWriter, r *http.Request) bool {
	if s.assetDir == "" {
		return false
	}
	p := filepath.Join(s.assetDir, theme, filepath.FromSlash(file))
	if _, err := os.Stat(p); err != nil {
		return false
	}
	mtype := assets.MimeTypeForFile(file)
	if len(mtype) != 0 {
		w.Header().Set("Content-Type", mtype)
	}
	http.ServeFile(w, r, p)
	return true
}

func (s *staticsServer) serveFromAssets(file, theme string, modificationTime time.Time, w http.ResponseWriter, r *http.Request) bool {
	as, ok := s.assets[theme+"/"+file]
	if !ok {
		return false
	}
	as.Modified = modificationTime
	assets.Serve(w, r, as)
	return true
}

func (s *staticsServer) serveThemes(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, map[string][]string{
		"themes": s.availableThemes,
	})
}

func (s *staticsServer) setTheme(theme string) {
	s.mut.Lock()
	s.theme = theme
	s.lastThemeChange = time.Now().UTC()
	s.mut.Unlock()
}

func (s *staticsServer) String() string {
	return fmt.Sprintf("staticsServer@%p", s)
}
