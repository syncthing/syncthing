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
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/text/unicode/norm"
)

const (
	// How long to consider cached dirnames valid
	caseCacheTimeout   = time.Second
	caseCacheItemLimit = 4 << 10
)

type ErrCaseConflict struct {
	Given, Real string
}

func (e *ErrCaseConflict) Error() string {
	return fmt.Sprintf(`remote "%v" uses different upper or lowercase characters than local "%v"; change the casing on either side to match the other`, e.Given, e.Real)
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
	fstype    FilesystemType
	uri, opts string
}

// caseFilesystemRegistry caches caseFilesystems and runs a routine to drop
// their cache every now and then.
type caseFilesystemRegistry struct {
	fss          map[fskey]*caseFilesystem
	mut          sync.RWMutex
	startCleaner sync.Once
}

func newFSKey(fs Filesystem) fskey {
	k := fskey{
		fstype: fs.Type(),
		uri:    fs.URI(),
	}
	if opts := fs.Options(); len(opts) > 0 {
		k.opts = opts[0].String()
		for _, o := range opts[1:] {
			k.opts += "&" + o.String()
		}
	}
	return k
}

func (r *caseFilesystemRegistry) get(fs Filesystem) Filesystem {
	k := newFSKey(fs)

	// Use double locking when getting a caseFs. In the common case it will
	// already exist and we take the read lock fast path. If it doesn't, we
	// take a write lock and try again.

	r.mut.RLock()
	caseFs, ok := r.fss[k]
	r.mut.RUnlock()

	if !ok {
		r.mut.Lock()
		caseFs, ok = r.fss[k]
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
		r.mut.Unlock()
	}

	return caseFs
}

func (r *caseFilesystemRegistry) cleaner() {
	for range time.NewTicker(time.Minute).C {
		// We need to not hold this lock for a long time, as it blocks
		// creating new filesystems in get(), which is needed to do things
		// like add new folders. The (*caseFs).dropCache() method can take
		// an arbitrarily long time to kick in because it in turn waits for
		// locks held by things performing I/O. So we can't call that from
		// within the loop.

		r.mut.RLock()
		toProcess := make([]*caseFilesystem, 0, len(r.fss))
		for _, caseFs := range r.fss {
			toProcess = append(toProcess, caseFs)
		}
		r.mut.RUnlock()

		for _, caseFs := range toProcess {
			caseFs.dropCache()
		}
	}
}

var globalCaseFilesystemRegistry = caseFilesystemRegistry{fss: make(map[fskey]*caseFilesystem)}

// OptionDetectCaseConflicts ensures that the potentially case-insensitive filesystem
// behaves like a case-sensitive filesystem. Meaning that it takes into account
// the real casing of a path and returns ErrCaseConflict if the given path differs
// from the real path. It is safe to use with any filesystem, i.e. also a
// case-sensitive one. However it will add some overhead and thus shouldn't be
// used if the filesystem is known to already behave case-sensitively.
type OptionDetectCaseConflicts struct{}

func (o *OptionDetectCaseConflicts) apply(fs Filesystem) Filesystem {
	return globalCaseFilesystemRegistry.get(fs)
}

func (o *OptionDetectCaseConflicts) String() string {
	return "detectCaseConflicts"
}

// caseFilesystem is a BasicFilesystem with additional checks to make a
// potentially case insensitive underlying FS behave like it's case-sensitive.
type caseFilesystem struct {
	Filesystem
	realCaser
}

func (f *caseFilesystem) Chmod(name string, mode FileMode) error {
	if err := f.checkCase(name); err != nil {
		return err
	}
	return f.Filesystem.Chmod(name, mode)
}

func (f *caseFilesystem) Lchown(name, uid, gid string) error {
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
	if err := f.checkCase(newpath); err != nil {
		// Case-only rename is ok
		e := &ErrCaseConflict{}
		if !errors.As(err, &e) || e.Real != oldpath {
			return err
		}
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

func (f *caseFilesystem) underlying() (Filesystem, bool) {
	return f.Filesystem, true
}

func (f *caseFilesystem) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeCase
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
	// We normalize the normalization (hah!) of the strings before
	// comparing, as we don't want to treat a normalization difference as a
	// case conflict.
	if norm.NFC.String(realName) != norm.NFC.String(name) {
		return &ErrCaseConflict{name, realName}
	}
	return nil
}

type defaultRealCaser struct {
	cache caseCache
}

func newDefaultRealCaser(fs Filesystem) *defaultRealCaser {
	cache, err := lru.New2Q(caseCacheItemLimit)
	// New2Q only errors if given invalid parameters, which we don't.
	if err != nil {
		panic(err)
	}
	return &defaultRealCaser{
		cache: caseCache{
			fs:            fs,
			TwoQueueCache: cache,
		},
	}
}

func (r *defaultRealCaser) realCase(name string) (string, error) {
	realName := "."
	if name == realName {
		return realName, nil
	}

	for _, comp := range PathComponents(name) {
		node := r.cache.getExpireAdd(realName)

		if node.err != nil {
			return "", node.err
		}

		// Try to find a direct or case match
		if _, ok := node.children[comp]; !ok {
			comp, ok = node.lowerToReal[UnicodeLowercaseNormalized(comp)]
			if !ok {
				return "", ErrNotExist
			}
		}

		realName = filepath.Join(realName, comp)
	}

	return realName, nil
}

func (r *defaultRealCaser) dropCache() {
	r.cache.Purge()
}

type caseCache struct {
	*lru.TwoQueueCache
	fs  Filesystem
	mut sync.Mutex
}

// getExpireAdd gets an entry for the given key. If no entry exists, or it is
// expired a new one is created and added to the cache.
func (c *caseCache) getExpireAdd(key string) *caseNode {
	c.mut.Lock()
	defer c.mut.Unlock()
	v, ok := c.Get(key)
	if !ok {
		node := newCaseNode(key, c.fs)
		c.Add(key, node)
		return node
	}
	node := v.(*caseNode)
	if node.expires.Before(time.Now()) {
		node = newCaseNode(key, c.fs)
		c.Add(key, node)
	}
	return node
}

// The keys to children are "real", case resolved names of the path
// component this node represents (i.e. containing no path separator).
// lowerToReal is a map of lowercase path components (as in UnicodeLowercase)
// to their corresponding "real", case resolved names.
// A node is created empty and populated using once. If an error occurs the node
// is removed from cache and the error stored in err, such that anyone that
// already got the node doesn't try to access the nil maps.
type caseNode struct {
	expires     time.Time
	lowerToReal map[string]string
	children    map[string]struct{}
	err         error
}

func newCaseNode(name string, filesystem Filesystem) *caseNode {
	node := new(caseNode)
	dirNames, err := filesystem.DirNames(name)
	// Set expiry after calling DirNames in case this is super-slow
	// (e.g. dirs with many children on android)
	node.expires = time.Now().Add(caseCacheTimeout)
	if err != nil {
		node.err = err
		return node
	}

	num := len(dirNames)
	node.children = make(map[string]struct{}, num)
	node.lowerToReal = make(map[string]string, num)
	lastLower := ""
	for _, n := range dirNames {
		node.children[n] = struct{}{}
		lower := UnicodeLowercaseNormalized(n)
		if lower != lastLower {
			node.lowerToReal[lower] = n
			lastLower = n
		}
	}

	return node
}
