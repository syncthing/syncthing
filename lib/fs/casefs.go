// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// How long to consider cached dirnames valid
	caseCacheTimeout = time.Second
)

type ErrCaseConflict struct {
	Given, Real string
}

func (e *ErrCaseConflict) Error() string {
	return fmt.Sprintf(`given name "%v" differs from name in filesystem "%v"`, e.Given, e.Real)
}

func IsErrCaseConflict(err error) bool {
	e := &ErrCaseConflict{}
	return errors.As(err, &e)
}

type realCaser interface {
	realCase(name string) (string, error)
	dropCache()
}

type fskey struct {
	fstype FilesystemType
	uri    string
}

// caseFilesystemRegistry caches caseFilesystems and runs a routine to drop
// their cache every now and then.
type caseFilesystemRegistry struct {
	fss          map[fskey]*caseFilesystem
	mut          sync.Mutex
	startCleaner sync.Once
}

func (r *caseFilesystemRegistry) get(fs Filesystem) Filesystem {
	r.mut.Lock()
	defer r.mut.Unlock()

	k := fskey{fs.Type(), fs.URI()}
	caseFs, ok := r.fss[k]
	if !ok {
		caseFs = &caseFilesystem{
			Filesystem: fs,
			realCaser:  newDefaultRealCaser(fs),
		}
		r.fss[k] = caseFs
		r.startCleaner.Do(func() {
			go r.cleaner()
		})
	}

	return caseFs
}

func (r *caseFilesystemRegistry) cleaner() {
	for range time.NewTicker(time.Minute).C {
		r.mut.Lock()
		for _, caseFs := range r.fss {
			caseFs.dropCache()
		}
		r.mut.Unlock()
	}
}

var globalCaseFilesystemRegistry = caseFilesystemRegistry{fss: make(map[fskey]*caseFilesystem)}

// caseFilesystem is a BasicFilesystem with additional checks to make a
// potentially case insensitive underlying FS behave like it's case-sensitive.
type caseFilesystem struct {
	Filesystem
	realCaser
}

// NewCaseFilesystem ensures that the given, potentially case-insensitive filesystem
// behaves like a case-sensitive filesystem. Meaning that it takes into account
// the real casing of a path and returns ErrCaseConflict if the given path differs
// from the real path. It is safe to use with any filesystem, i.e. also a
// case-sensitive one. However it will add some overhead and thus shouldn't be
// used if the filesystem is known to already behave case-sensitively.
func NewCaseFilesystem(fs Filesystem) Filesystem {
	return wrapFilesystem(fs, globalCaseFilesystemRegistry.get)
}

func (f *caseFilesystem) Chmod(name string, mode FileMode) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.Filesystem.Chmod(name, mode)
}

func (f *caseFilesystem) Lchown(name string, uid, gid int) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.Filesystem.Lchown(name, uid, gid)
}

func (f *caseFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.Filesystem.Chtimes(name, atime, mtime)
}

func (f *caseFilesystem) Mkdir(name string, perm FileMode) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.Filesystem.Mkdir(name, perm); err != nil {
		return err
	}
	f.dropCache()
	return nil
}

func (f *caseFilesystem) MkdirAll(path string, perm FileMode) error {
	if err := f.checkCase(path); err != nil {
		return err
	}
	if err := f.Filesystem.MkdirAll(path, perm); err != nil {
		return err
	}
	f.dropCache()
	return nil
}

func (f *caseFilesystem) Lstat(name string) (FileInfo, error) {
	var err error
	if name, err = Canonicalize(name); err != nil {
		return nil, err
	}
	stat, err := f.Filesystem.Lstat(name)
	if err != nil {
		return nil, err
	}
	if err = f.checkCaseExisting(name); err != nil {
		return nil, err
	}
	return stat, nil
}

func (f *caseFilesystem) Remove(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.Filesystem.Remove(name); err != nil {
		return err
	}
	f.dropCache()
	return nil
}

func (f *caseFilesystem) RemoveAll(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.Filesystem.RemoveAll(name); err != nil {
		return err
	}
	f.dropCache()
	return nil
}

func (f *caseFilesystem) Rename(oldpath, newpath string) error {
	if err := f.checkCase(oldpath); err != nil {
		return err
	}
	if err := f.Filesystem.Rename(oldpath, newpath); err != nil {
		return err
	}
	f.dropCache()
	return nil
}

func (f *caseFilesystem) Stat(name string) (FileInfo, error) {
	var err error
	if name, err = Canonicalize(name); err != nil {
		return nil, err
	}
	stat, err := f.Filesystem.Stat(name)
	if err != nil {
		return nil, err
	}
	if err = f.checkCaseExisting(name); err != nil {
		return nil, err
	}
	return stat, nil
}

func (f *caseFilesystem) DirNames(name string) ([]string, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	return f.Filesystem.DirNames(name)
}

func (f *caseFilesystem) Open(name string) (File, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	return f.Filesystem.Open(name)
}

func (f *caseFilesystem) OpenFile(name string, flags int, mode FileMode) (File, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	file, err := f.Filesystem.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	f.dropCache()
	return file, nil
}

func (f *caseFilesystem) ReadSymlink(name string) (string, error) {
	if err := f.checkCase(name); err != nil {
		return "", err
	}
	return f.Filesystem.ReadSymlink(name)
}

func (f *caseFilesystem) Create(name string) (File, error) {
	if err := f.checkCase(name); err != nil {
		return nil, err
	}
	file, err := f.Filesystem.Create(name)
	if err != nil {
		return nil, err
	}
	f.dropCache()
	return file, nil
}

func (f *caseFilesystem) CreateSymlink(target, name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	if err := f.Filesystem.CreateSymlink(target, name); err != nil {
		return err
	}
	f.dropCache()
	return nil
}

func (f *caseFilesystem) Walk(root string, walkFn WalkFunc) error {
	// Walking the filesystem is likely (in Syncthing's case certainly) done
	// to pick up external changes, for which caching is undesirable.
	f.dropCache()
	if err := f.checkCase(root); err != nil {
		return err
	}
	return f.Filesystem.Walk(root, walkFn)
}

func (f *caseFilesystem) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	if err := f.checkCase(path); err != nil {
		return nil, nil, err
	}
	return f.Filesystem.Watch(path, ignore, ctx, ignorePerms)
}

func (f *caseFilesystem) Hide(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.Filesystem.Hide(name)
}

func (f *caseFilesystem) Unhide(name string) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.Filesystem.Unhide(name)
}

func (f *caseFilesystem) checkCase(name string) error {
	var err error
	if name, err = Canonicalize(name); err != nil {
		return err
	}
	// Stat is necessary for case sensitive FS, as it's then not a conflict
	// if name is e.g. "foo" and on dir there is "Foo".
	if _, err := f.Filesystem.Lstat(name); err != nil {
		if IsNotExist(err) {
			return nil
		}
		return err
	}
	return f.checkCaseExisting(name)
}

// checkCaseExisting must only be called after successfully canonicalizing and
// stating the file.
func (f *caseFilesystem) checkCaseExisting(name string) error {
	realName, err := f.realCase(name)
	if IsNotExist(err) {
		// It did exist just before -> cache is outdated, try again
		f.dropCache()
		realName, err = f.realCase(name)
	}
	if err != nil {
		return err
	}
	if realName != name {
		return &ErrCaseConflict{name, realName}
	}
	return nil
}

type defaultRealCaser struct {
	fs   Filesystem
	root *caseNode
	mut  sync.RWMutex
}

func newDefaultRealCaser(fs Filesystem) *defaultRealCaser {
	caser := &defaultRealCaser{
		fs:   fs,
		root: &caseNode{name: "."},
	}
	return caser
}

func (r *defaultRealCaser) realCase(name string) (string, error) {
	out := "."
	if name == out {
		return out, nil
	}

	r.mut.Lock()
	defer r.mut.Unlock()

	node := r.root
	for _, comp := range strings.Split(name, string(PathSeparator)) {
		if node.dirNames == nil || node.expires.Before(time.Now()) {
			// Haven't called DirNames yet, or the node has expired

			var err error
			node.dirNames, err = r.fs.DirNames(out)
			if err != nil {
				return "", err
			}

			node.dirNamesLower = make([]string, len(node.dirNames))
			for i, n := range node.dirNames {
				node.dirNamesLower[i] = UnicodeLowercase(n)
			}

			node.expires = time.Now().Add(caseCacheTimeout)
			node.child = nil
		}

		// If we don't already have a correct cached child, try to find it.
		if node.child == nil || node.child.name != comp {
			// Actually loop dirNames to search for a match.
			n, err := findCaseInsensitiveMatch(comp, node.dirNames, node.dirNamesLower)
			if err != nil {
				return "", err
			}
			node.child = &caseNode{name: n}
		}

		node = node.child
		out = filepath.Join(out, node.name)
	}

	return out, nil
}

func (r *defaultRealCaser) dropCache() {
	r.mut.Lock()
	r.root = &caseNode{name: "."}
	r.mut.Unlock()
}

// Both name and the key to children are "Real", case resolved names of the path
// component this node represents (i.e. containing no path separator).
// The key to results is also a path component, but as given to RealCase, not
// case resolved.
type caseNode struct {
	name          string
	expires       time.Time
	dirNames      []string
	dirNamesLower []string
	child         *caseNode
}

func findCaseInsensitiveMatch(name string, names, namesLower []string) (string, error) {
	lower := UnicodeLowercase(name)
	candidate := ""
	for i, n := range names {
		if n == name {
			return n, nil
		}
		if candidate == "" && namesLower[i] == lower {
			candidate = n
		}
	}
	if candidate == "" {
		return "", ErrNotExist
	}
	return candidate, nil
}
