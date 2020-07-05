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
	"time"
)

// Both values were chosen by magic.
const (
	caseCacheTimeout = 10 * time.Second
	// When the number of names (all lengths of []string from DirNames)
	// exceeds this, we drop the cache.
	caseMaxCachedNames = 1 << 20
)

type basicCachedRealCaser struct {
	fs        *BasicFilesystem
	root      *caseNode
	nameCount int
	lastClean time.Time
}

func newBasicCachedRealCaser(fs *BasicFilesystem) *basicCachedRealCaser {
	return &basicCachedRealCaser{
		fs:        fs,
		lastClean: time.Now(),
		root:      &caseNode{name: "."},
	}
}

// RealCase returns the correct case insensitive match for the given name.
// Important drawback: If the filesystem is case sensitive, it will also return
// a case insensitive match. E.g. if `Foo` exists on disk and name is 'foo', it
// will return `Foo`. It still returns the correct match: If both `Foo` and `foo`
// exist, and name is `Foo`, it will return `Foo` and never `foo`.
func (c *basicCachedRealCaser) RealCase(name string) (string, error) {
	if c.nameCount > caseMaxCachedNames || time.Since(c.lastClean) > caseCacheTimeout {
		c.root = &caseNode{name: "."}
		c.lastClean = time.Now()
		c.nameCount = 0
	}

	var err error
	name, err = Canonicalize(name)
	if err != nil {
		return "", err
	}

	out := "."
	if name == out {
		return out, nil
	}

	pathComponents := strings.Split(name, string(PathSeparator))

	node := c.root
	for _, comp := range pathComponents {
		if node.dirNames == nil {
			// Haven't called DirNames yet
			node.dirNames, err = c.fs.DirNames(out)
			if err != nil {
				return "", err
			}
			node.children = make(map[string]*caseNode)
			node.results = make(map[string]*caseNode)
			c.nameCount += len(node.dirNames)
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
