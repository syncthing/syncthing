// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"net/http"
	"sync"
)

// BundledApp represents a connected application that has registered itself
// with Syncthing.
type BundledApp struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
}

// bundledService manages registered applications
type bundledService struct {
	mu    sync.RWMutex
	apps  map[string]BundledApp
}

var bundled = &bundledService{
	apps: make(map[string]BundledApp),
}

// RegisterBundledApp registers or updates a bundled application
func RegisterBundledApp(name, version, url string) {
	bundled.mu.Lock()
	defer bundled.mu.Unlock()
	bundled.apps[name] = BundledApp{
		Name:    name,
		Version: version,
		URL:     url,
	}
}

// GetBundledApps returns all registered bundled applications
func GetBundledApps() []BundledApp {
	bundled.mu.RLock()
	defer bundled.mu.RUnlock()
	apps := make([]BundledApp, 0, len(bundled.apps))
	for _, app := range bundled.apps {
		apps = append(apps, app)
	}
	return apps
}

func (s *service) getSystemBundled(w http.ResponseWriter, _ *http.Request) {
	sendJSON(w, GetBundledApps())
}
