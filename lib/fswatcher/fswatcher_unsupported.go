// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build solaris,!cgo darwin,!cgo

package fswatcher

import (
	"fmt"
	"runtime"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/ignore"
)

type fsWatcher struct {
}

type Service interface {
	Serve()
	Stop()
	FsWatchChan() <-chan []string
	UpdateIgnores(ignores *ignore.Matcher)
}

func NewFsWatcher(cfg config.FolderConfiguration, ignores *ignore.Matcher) (Service, error) {
	err := fmt.Errorf("not available on %v-%v", runtime.GOOS, runtime.GOARCH)
	l.Warnln("Filesystem notifications:", err)
	return nil, err
}

func (watcher *fsWatcher) Serve() {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) Stop() {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) FsWatchChan() <-chan []string {
	panic("bug: should never be called")
}

func (watcher *fsWatcher) UpdateIgnores(ignores *ignore.Matcher) {
	panic("bug: should never be called")
}
