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
	"path/filepath"
	"strings"

	"github.com/zillode/notify"
)

// Notify does not block on sending to channel, so the channel must be buffered.
// The actual number is magic.
// Not meant to be changed, but must be changeable for tests
var backendBuffer = 500

func (f *BasicFilesystem) Watch(name string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, error) {
	absName, err := f.rooted(name)
	if err != nil {
		return nil, err
	}

	absShouldIgnore := func(absPath string) bool {
		if !isInsideRoot(absPath, absName) {
			panic("bug: Notify backend is processing a change outside of the watched path: " + absPath)
		}
		relPath, _ := filepath.Rel(absName, absPath)
		return ignore.ShouldIgnore(relPath)
	}

	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)

	eventMask := subEventMask
	if !ignorePerms {
		eventMask |= permEventMask
	}

	if err := notify.WatchWithFilter(filepath.Join(absName, "..."), backendChan, absShouldIgnore, eventMask); err != nil {
		notify.Stop(backendChan)
		if reachedMaxUserWatches(err) {
			err = errors.New("failed to install inotify handler. Please increase inotify limits, see https://github.com/syncthing/syncthing-inotify#troubleshooting-for-folders-with-many-files-on-linux for more information")
		}
		return nil, err
	}

	go f.watchLoop(absName, backendChan, outChan, ignore, ctx)

	return outChan, nil
}

func (f *BasicFilesystem) watchLoop(absName string, backendChan chan notify.EventInfo, outChan chan<- Event, ignore Matcher, ctx context.Context) {
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
			outChan <- Event{Name: ".", Type: NonRemove}
			l.Debugln(f.Type(), f.URI(), "Watch: Event overflow, send \".\"")
		}

		select {
		case ev := <-backendChan:
			if !isInsideRoot(ev.Path(), absName) {
				panic("bug: BasicFilesystem watch received event outside of the watched path: " + ev.Path())
			}
			relPath, _ := filepath.Rel(absName, ev.Path())
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

// The added separator is necessary, as root has a separator attached while
// path must be identical to the return value of filepath.Clean(path) (i.e.
// does not have a separator attached).
func isInsideRoot(path string, root string) bool {
	return strings.HasPrefix(path+string(PathSeparator), root)
}
