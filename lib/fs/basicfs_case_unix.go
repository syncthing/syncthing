// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Both values were chosen by magic.
const (
	caseCacheTimeout = 10 * time.Second
	// When the number of names (all lengths of []string from DirNames)
	// exceeds this, we drop the cache.
	caseMaxCachedNames = 1 << 20
)

var casers = make(map[string]*realCaser)

type realCaser struct {
	fs            *BasicFilesystem
	caseRoot      *caseNode
	caseCount     int
	caseTimer     *time.Timer
	caseTimerStop chan struct{}
	caseMut       sync.RWMutex
}

func newRealCaser(fs *BasicFilesystem) *realCaser {
	caser, ok := casers[fs.root]
	if ok {
		return caser
	}
	caser = &realCaser{
		fs:        fs,
		caseRoot:  &caseNode{name: "."},
		caseTimer: time.NewTimer(0),
	}
	<-caser.caseTimer.C
	casers[fs.root] = caser
	return caser
}

func (r *realCaser) realCase(name string) (string, error) {
	out := "."
	if name == out {
		return out, nil
	}

	r.caseMut.Lock()
	defer func() {
		if r.caseCount > caseMaxCachedNames {
			select {
			case r.caseTimerStop <- struct{}{}:
			default:
			}
			r.cleanCaseLocked()
		} else if r.caseTimerStop == nil {
			r.startCaseResetTimerLocked()
		}
		r.caseMut.Unlock()
	}()

	node := r.caseRoot
	for _, comp := range strings.Split(name, string(PathSeparator)) {
		if node.dirNames == nil {
			// Haven't called DirNames yet
			var err error
			node.dirNames, err = r.fs.DirNames(out)
			if err != nil {
				return "", err
			}
			node.children = make(map[string]*caseNode)
			node.results = make(map[string]*caseNode)
			r.caseCount += len(node.dirNames)
		} else if child, ok := node.results[comp]; ok {
			// Check if this exact name has been queried before to shortcut
			node = child
			out = filepath.Join(out, child.name)
			continue
		}
		// Actually loop dirNames to search for a match
		n, err := findCaseInsensitiveMatch(comp, node.dirNames)
		if err != nil {
			return "", err
		}
		child, ok := node.children[n]
		if !ok {
			child = &caseNode{name: n}
		}
		node.results[comp] = child
		node.children[n] = child
		node = child
		out = filepath.Join(out, n)
	}

	return out, nil
}

func (r *realCaser) startCaseResetTimerLocked() {
	r.caseTimerStop = make(chan struct{})
	r.caseTimer.Reset(caseCacheTimeout)
	go func() {
		select {
		case <-r.caseTimer.C:
			r.cleanCase()
		case <-r.caseTimerStop:
			if !r.caseTimer.Stop() {
				<-r.caseTimer.C
			}
			r.caseMut.Lock()
			r.caseTimerStop = nil
			r.caseMut.Unlock()
		}
	}()
}

func (r *realCaser) cleanCase() {
	r.caseMut.Lock()
	r.cleanCaseLocked()
	r.caseMut.Unlock()
}

func (r *realCaser) cleanCaseLocked() {
	r.caseRoot = &caseNode{name: "."}
	r.caseCount = 0
}

// Both name and the key to children are "Real", case resolved names of the path
// component this node represents (i.e. containing no path separator).
// The key to results is also a path component, but as given to RealCase, not
// case resolved.
type caseNode struct {
	name     string
	dirNames []string
	children map[string]*caseNode
	results  map[string]*caseNode
}

func findCaseInsensitiveMatch(name string, names []string) (string, error) {
	lower := UnicodeLowercase(name)
	candidate := ""
	for _, n := range names {
		if n == name {
			return n, nil
		}
		if candidate == "" && UnicodeLowercase(n) == lower {
			candidate = n
		}
	}
	if candidate == "" {
		return "", ErrNotExist
	}
	return candidate, nil
}
