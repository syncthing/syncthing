// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This part copied directly from golang.org/src/path/filepath/path.go (Go
// 1.6) and lightly modified to be methods on BasicFilesystem.

// In our Walk() all paths given to a WalkFunc() are relative to the
// filesystem root.

package fs

import (
	"container/heap"
	"errors"
	"path/filepath"
)

var ErrInfiniteRecursion = errors.New("infinite filesystem recursion detected")

// walkEntry represents a file/directory to be visited
type walkEntry struct {
	path string
	info FileInfo
}

// pathHeap implements heap.Interface for lexicographic ordering by path
type pathHeap []walkEntry

func (h pathHeap) Len() int           { return len(h) }
func (h pathHeap) Less(i, j int) bool { return h[i].path < h[j].path }
func (h pathHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *pathHeap) Push(x any) {
	*h = append(*h, x.(walkEntry))
}

func (h *pathHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
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

	checkInfiniteRecursion bool
}

func NewWalkFilesystem(next Filesystem) Filesystem {
	fs := &walkFilesystem{
		Filesystem: next,
	}
	for _, opt := range next.Options() {
		if _, ok := opt.(*OptionJunctionsAsDirs); ok {
			fs.checkInfiniteRecursion = true
			break
		}
	}
	return fs
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn.
//
// IMPORTANT: Files are walked in lexicographic order by FULL PATH, matching
// DB ORDER BY name. This uses a min-heap to ensure correct ordering where
// "a.d/x" comes before "a/x" (since '.' < '/').
//
// Memory complexity: O(W) where W = max entries pending in heap at any time.
//
//   - Typical case: W ≈ directory_width × tree_depth ≈ 100-1000 entries.
//     Most directories have 10-100 items, heap stays small.
//
//   - Worst case: W = max(directory_entries). If one directory has 10K files,
//     heap holds 10K entries while processing that directory.
//
// This is NOT O(n) where n = total files, unless all files are in one flat directory.
func (f *walkFilesystem) Walk(root string, walkFn WalkFunc) error {
	root, err := Canonicalize(root)
	if err != nil {
		return err
	}

	info, err := f.Lstat(root)
	if err != nil {
		return walkFn(root, nil, err)
	}

	// Initialize heap with root
	h := &pathHeap{}
	heap.Init(h)

	// For root ".", we need to handle it specially
	if root == "." {
		// Call walkFn for root
		if err := walkFn(root, info, nil); err != nil {
			if errors.Is(err, SkipDir) {
				return nil
			}
			return err
		}

		// If root is a directory, add its children to heap
		if info.IsDir() {
			entries, err := f.ReadDir(root)
			if err != nil {
				return walkFn(root, info, err)
			}
			for _, entry := range entries {
				childPath := entry.Name()
				childInfo, err := f.Lstat(childPath)
				if err != nil {
					if err := walkFn(childPath, nil, err); err != nil && !errors.Is(err, SkipDir) {
						return err
					}
					continue
				}
				heap.Push(h, walkEntry{path: childPath, info: childInfo})
			}
		}
	} else {
		heap.Push(h, walkEntry{path: root, info: info})
	}

	// Track visited directories for infinite recursion detection
	var visited map[string]bool
	if f.checkInfiniteRecursion {
		visited = make(map[string]bool)
	}

	// Process entries in lexicographic order
	for h.Len() > 0 {
		entry := heap.Pop(h).(walkEntry)

		// Check for infinite recursion
		if f.checkInfiniteRecursion && entry.info.IsDir() {
			// Use inode-based detection would be better, but path works for now
			if visited[entry.path] {
				if err := walkFn(entry.path, entry.info, ErrInfiniteRecursion); err != nil {
					return err
				}
				continue
			}
			visited[entry.path] = true
		}

		// Call walkFn
		err := walkFn(entry.path, entry.info, nil)
		if err != nil {
			if errors.Is(err, SkipDir) {
				// Skip this directory's contents
				continue
			}
			return err
		}

		// If directory, add children to heap
		if entry.info.IsDir() {
			entries, err := f.ReadDir(entry.path)
			if err != nil {
				if err := walkFn(entry.path, entry.info, err); err != nil && !errors.Is(err, SkipDir) {
					return err
				}
				continue
			}

			for _, dirEntry := range entries {
				childPath := filepath.Join(entry.path, dirEntry.Name())
				childInfo, err := f.Lstat(childPath)
				if err != nil {
					if err := walkFn(childPath, nil, err); err != nil && !errors.Is(err, SkipDir) {
						return err
					}
					continue
				}
				heap.Push(h, walkEntry{path: childPath, info: childInfo})
			}
		}
	}

	return nil
}

func (f *walkFilesystem) underlying() (Filesystem, bool) {
	return f.Filesystem, true
}
