// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build solaris,!cgo darwin,!cgo

package fswatcher

import (
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/ignore"
)

type watcher struct {
}

type Service interface {
	Serve()
	Stop()
	C() <-chan []string
	UpdateIgnores(ignores *ignore.Matcher)
	VerifyConfiguration(from, to config.Configuration) error
	CommitConfiguration(from, to config.Configuration) bool
	String() string
}

func New(folderCfg config.FolderConfiguration, cfg *config.Wrapper, ignores *ignore.Matcher) Service {
	return nil
}

var panicMsg = "bug: should never be called"

func (w *watcher) Serve() {
	panic(panicMsg)
}

func (w *watcher) Stop() {
	panic(panicMsg)
}

func (w *watcher) C() <-chan []string {
	panic(panicMsg)
}

func (w *watcher) UpdateIgnores(ignores *ignore.Matcher) {
	panic(panicMsg)
}

func (w *watcher) VerifyConfiguration(from, to config.Configuration) error {
	panic(panicMsg)
}

func (w *watcher) CommitConfiguration(from, to config.Configuration) bool {
	panic(panicMsg)
}

func (w *watcher) String() string {
	panic(panicMsg)
}
