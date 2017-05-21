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
	err := fmt.Errorf("Filesystem notifications are not available on "+
		"%v-%v due to incompatibility of cgo with cross-compilation.",
		runtime.GOOS, runtime.GOARCH)
	l.Warnf(fmt.Sprintln(err))
	return nil, err
}

func (watcher *fsWatcher) Serve() {
	panic("This should haven never been called, as it should be " +
		"impossible to create a fswatcher.Service instance")
}

func (watcher *fsWatcher) Stop() {
	panic("This should haven never been called, as it should be " +
		"impossible to create a fswatcher.Service instance")
}

func (watcher *fsWatcher) FsWatchChan() <-chan []string {
	panic("This should haven never been called, as it should be " +
		"impossible to create a fswatcher.Service instance")
}

func (watcher *fsWatcher) UpdateIgnores(ignores *ignore.Matcher) {
	panic("This should haven never been called, as it should be " +
		"impossible to create a fswatcher.Service instance")
}
