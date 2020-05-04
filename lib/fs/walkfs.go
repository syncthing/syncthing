// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This part copied directly from golang.org/src/path/filepath/path.go (Go
// 1.6) and lightly modified to be methods on BasicFilesystem.

// In our Walk() all paths given to a WalkFunc() are relative to the
// filesystem root.

package fs

import (
	"fmt"
	"path/filepath"
)

func newAncestorDirList() *ancestorDirList {
	l.Debugf("ancestorDirList: new")
	return &ancestorDirList{list: make([]FileInfo, 0, 20)}
}

type ancestorDirList struct {
	list []FileInfo
}

func (ancestors *ancestorDirList) AppendUnlessPresent(info FileInfo) bool {
	l.Debugf("ancestorDirList: AppendUnlessPresent '%s'", info.Name())
	for _, ancestor := range ancestors.list {
		if SameFile(info, ancestor) {
			return false
		}
	}
	aLen := len(ancestors.list)
	if aLen == cap(ancestors.list) {
		t := make([]FileInfo, aLen, aLen*2)
		copy(t, ancestors.list)
		ancestors.list = t
		l.Debugf("ancestorDirList: capacity increased to %d", cap(ancestors.list))
	}
	ancestors.list = append(ancestors.list, info)
	return true
}

func (ancestors *ancestorDirList) Remove(info FileInfo) bool {
	l.Debugf("ancestorDirList: Remove '%s'", info.Name())
	aLen := len(ancestors.list)
	if aLen == 0 {
		panic(fmt.Sprintf("Removal of item '%s' attempted on empty ancestorDirList", info.Name()))
		//return false
	} else if info.Name() != ancestors.list[aLen-1].Name() { //!SameFile(info, ancestors.list[aLen-1]) may fail
		panic(fmt.Sprintf("ancestorDirList.Remove attempted for item '%s', but the topmost ancestor is '%s'", info.Name(), ancestors.list[aLen-1]))
		//return false
	}
	ancestors.list = ancestors.list[:aLen-1]
	return true
}

func (ancestors *ancestorDirList) Count() int {
	return len(ancestors.list)
}

// WalkFunc is the type of the function called for each file or directory
// visited by Walk. The path argument contains the argument to Walk as a
// prefix; that is, if Walk is called with "dir", which is a directory
// containing the file "a", the walk function will be called with argument
// "dir/a". The info argument is the FileInfo for the named path.
//
// If there was a problem walking to the file or directory named by path, the
// incoming error will describe the problem and the function can decide how
// to handle that error (and Walk will not descend into that directory). If
// an error is returned, processing stops. The sole exception is when the function
// returns the special value SkipDir. If the function returns SkipDir when invoked
// on a directory, Walk skips the directory's contents entirely.
// If the function returns SkipDir when invoked on a non-directory file,
// Walk skips the remaining files in the containing directory.
type WalkFunc func(path string, info FileInfo, err error) error

type walkFilesystem struct {
	Filesystem
}

func NewWalkFilesystem(next Filesystem) Filesystem {
	return &walkFilesystem{next}
}

// walk recursively descends path, calling walkFn.
func (f *walkFilesystem) walk(path string, info FileInfo, walkFn WalkFunc, ancestors *ancestorDirList) error {
	l.Debugf("walk: path=%s", path)
	path, err := Canonicalize(path)
	if err != nil {
		return err
	}

	err = walkFn(path, info, nil)
	if err != nil {
		if info.IsDir() && err == SkipDir {
			return nil
		}
		return err
	}

	if !info.IsDir() && path != "." {
		return nil
	}

	if ancestors.AppendUnlessPresent(info) {
		defer ancestors.Remove(info)
	} else {
		l.Warnf("Infinite filesystem recursion detected on path '%s', not walking further down", path)
		return nil
	}

	names, err := f.DirNames(path)
	if err != nil {
		return walkFn(path, info, err)
	}

	for _, name := range names {
		filename := filepath.Join(path, name)
		fileInfo, err := f.Lstat(filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != SkipDir {
				return err
			}
		} else {
			err = f.walk(filename, fileInfo, walkFn, ancestors)
			if err != nil {
				if !fileInfo.IsDir() || err != SkipDir {
					return err
				}
			}
		}
	}
	return nil
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The files are walked in lexical
// order, which makes the output deterministic but means that for very
// large directories Walk can be inefficient.
// Walk does not follow symbolic links.
func (f *walkFilesystem) Walk(root string, walkFn WalkFunc) error {
	info, err := f.Lstat(root)
	if err != nil {
		return walkFn(root, nil, err)
	}
	ancestors := newAncestorDirList()
	err = f.walk(root, info, walkFn, ancestors)
	if ancestors.Count() != 0 {
		panic(fmt.Sprintf("fs.Walk finished with %d unremoved ancestors", ancestors.Count()))
	}
	return err
}
