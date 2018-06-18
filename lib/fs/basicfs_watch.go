// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/syncthing/notify"
)

// Notify does not block on sending to channel, so the channel must be buffered.
// The actual number is magic.
// Not meant to be changed, but must be changeable for tests
var backendBuffer = 500

func (f *BasicFilesystem) Watch(name string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, error) {
	absName, err := f.rootedSymlinkEvaluated(name)
	if err != nil {
		return nil, err
	}

	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)

	eventMask := subEventMask
	if !ignorePerms {
		eventMask |= permEventMask
	}

	if ignore.SkipIgnoredDirs() {
		absShouldIgnore := func(absPath string) bool {
			return ignore.ShouldIgnore(f.unrootedChecked(absPath))
		}
		err = notify.WatchWithFilter(filepath.Join(absName, "..."), backendChan, absShouldIgnore, eventMask)
	} else {
		err = notify.Watch(filepath.Join(absName, "..."), backendChan, eventMask)
	}
	if err != nil {
		notify.Stop(backendChan)
		if reachedMaxUserWatches(err) {
			err = errors.New("failed to setup inotify handler. Please increase inotify limits, see https://docs.syncthing.net/users/faq.html#inotify-limits")
		}
		return nil, err
	}

	go f.watchLoop(name, backendChan, outChan, ignore, ctx)

	return outChan, nil
}

func (f *BasicFilesystem) watchLoop(name string, backendChan chan notify.EventInfo, outChan chan<- Event, ignore Matcher, ctx context.Context) {
	for {
		// Detect channel overflow
		if len(backendChan) == backendBuffer {
		outer:
			for {
				select {
				case <-backendChan:
				default:
					break outer
				}
			}
			// When next scheduling a scan, do it on the entire folder as events have been lost.
			outChan <- Event{Name: name, Type: NonRemove}
			l.Debugln(f.Type(), f.URI(), "Watch: Event overflow, send \".\"")
		}

		select {
		case ev := <-backendChan:
			relPath := f.unrootedChecked(ev.Path())
			if ignore.ShouldIgnore(relPath) {
				l.Debugln(f.Type(), f.URI(), "Watch: Ignoring", relPath)
				continue
			}
			evType := f.eventType(ev.Event())
			select {
			case outChan <- Event{Name: relPath, Type: evType}:
				l.Debugln(f.Type(), f.URI(), "Watch: Sending", relPath, evType)
			case <-ctx.Done():
				notify.Stop(backendChan)
				l.Debugln(f.Type(), f.URI(), "Watch: Stopped")
				return
			}
		case <-ctx.Done():
			notify.Stop(backendChan)
			l.Debugln(f.Type(), f.URI(), "Watch: Stopped")
			return
		}
	}
}

func (f *BasicFilesystem) eventType(notifyType notify.Event) EventType {
	if notifyType&rmEventMask != 0 {
		return Remove
	}
	return NonRemove
}

// unrootedChecked returns the path relative to the folder root (same as
// unrooted). It panics if the given path is not a subpath and handles the
// special case when the given path is the folder root without a trailing
// pathseparator.
func (f *BasicFilesystem) unrootedChecked(absPath string) string {
	if absPath+string(PathSeparator) == f.rootSymlinkEvaluated {
		return "."
	}
	if !strings.HasPrefix(absPath, f.rootSymlinkEvaluated) {
		panic(fmt.Sprintf("bug: Notify backend is processing a change outside of the filesystem root: root==%v, rootSymEval==%v, path==%v", f.root, f.rootSymlinkEvaluated, absPath))
	}
	return f.unrootedSymlinkEvaluated(f.resolveWin83(absPath))
}
