// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build solaris,!cgo darwin,!cgo

package fswatcher

import (
	"runtime"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/ignore"
)

type fsWatcher struct {
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

func NewFsWatcher(id string, cfg *config.Wrapper, ignores *ignore.Matcher) (Service, error) {
	return nil, fmt.Errorf("not available on %v-%v", runtime.GOOS, runtime.GOARCH)
}

func (watcher *fsWatcher) Serve() {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) Stop() {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) C() <-chan []string {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) UpdateIgnores(ignores *ignore.Matcher) {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) VerifyConfiguration(from, to config.Configuration) error {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) CommitConfiguration(from, to config.Configuration) bool {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) String() string {
	panic("bug: should never be called")
}
