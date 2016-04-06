// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// The TempNamer provides naming for temporary files.
type TempNamer interface {
	TempName(path string) string
}

// The CurrentFiler provides access to the current state of a file.
type CurrentFiler interface {
	CurrentFile(name string) (protocol.FileInfo, bool)
}

// The Requester responds to requests for blocks by filling out the buffer or
// returning an error.
type Requester interface {
	Request(file string, offset int64, hash []byte, buf []byte) error
}

// The Archiver can elect to archive a file just before it would otherwise be
// replaced or removed.
type Archiver interface {
	Archive(path string) error
}

// The Progresser receives progress information along the way. Each queued
// item gets passed exactly once to Started and Completed (in that order),
// and zero or more times to Progress in between that. Calls to Progress will
// follow the pattern:
//
// Progress(f, n, 0, 0) where n > 0: a block was copied (n is the size of the
// block)
//
// Progress(f, 0, n, 0) where n > 0: a block was requested from the network
//
// Progress(f, 0, -n, n) where n > 0: a block (that was previously requested)
// was received from the network
//
// Progress(f, 0, -n, 0) where n > 0: a block (that was previously requested)
// failed to download
//
// As such, if a sum of each parameter is kept, at any given time the sum of
// copied+downloaded represents the amount of data successfully retrieved,
// and the sum of requested is the amount of data that is outstanding in
// requests.
//
// The Progresser is called concurrently from multiple goroutines.
type Progresser interface {
	Started(file protocol.FileInfo)
	Progress(file protocol.FileInfo, copied, requested, downloaded int)
	Completed(file protocol.FileInfo, err error)
}

// An ApplyError is returned by Apply when a change set cannot be applied.
type ApplyError interface {
	error

	// MustRescan returns true if the cause of the error is that the index
	// database is out of sync with the on disk contents.
	MustRescan() bool
	// Errors returns the list of individial errors (each an opError) that
	// ocurred.
	Errors() []error
}

type Options struct {
	RootPath         string
	MaxConflicts     int
	TempNamer        TempNamer       // needed if we do any operations on files or symlinks
	LocalRequester   Requester       // needed to reuse blocks locally
	NetworkRequester *AsyncRequester // needed to request blocks from the network
	CurrentFiler     CurrentFiler    // used for conflict detection and metadata shortcut
	Archiver         Archiver        // used for archiving old versions
	Filesystem       fs.Filesystem   // handles low level filesystem operations
	Progresser       Progresser      // used to report per file progress data and completion

}

type ChangeSet struct {
	// Set by the constructor
	rootPath     string
	maxConflicts int // 0 disables conflict copies, -1 gives unlimited conflict copies

	// Somewhat optional attributes
	tempNamer        TempNamer
	localRequester   Requester
	networkRequester *AsyncRequester
	currentFiler     CurrentFiler
	archiver         Archiver
	filesystem       fs.Filesystem
	progresser       Progresser

	mut           sync.Mutex
	queue         []fileInfo
	deletedHashes map[string]protocol.FileInfo
}

type fileInfo struct {
	protocol.FileInfo
	synthetic bool // We made this update up ourselves, it should not be reported to the outside.
}

func (f fileInfo) String() string {
	return fmt.Sprintf("%v (synth=%v)", f.FileInfo, f.synthetic)
}

// New creates and returns a new empty ChangeSet.
func New(opts Options) *ChangeSet {
	c := &ChangeSet{
		rootPath:         opts.RootPath,
		maxConflicts:     opts.MaxConflicts,
		tempNamer:        opts.TempNamer,
		localRequester:   opts.LocalRequester,
		networkRequester: opts.NetworkRequester,
		currentFiler:     opts.CurrentFiler,
		archiver:         opts.Archiver,
		filesystem:       opts.Filesystem,
		progresser:       opts.Progresser,

		deletedHashes: make(map[string]protocol.FileInfo),
		mut:           sync.NewMutex(),
	}

	// Set some defaults if they were not set from the opts.
	if c.filesystem == nil {
		c.filesystem = fs.DefaultFilesystem
	}
	if c.progresser == nil {
		c.progresser = nilProgresser{}
	}

	return c
}

// Queue adds a file (or directory) to the change set. In case of a deletion,
// the FileInfo should be modified to contain the block list of the *current*
// file.
func (c *ChangeSet) Queue(f protocol.FileInfo) {
	c.mut.Lock()
	defer c.mut.Unlock()

	c.queue = append(c.queue, fileInfo{FileInfo: f})

	if f.IsDeleted() && !(f.IsDirectory() || f.IsSymlink()) {
		key := string(f.Hash())
		f.Blocks = nil
		c.deletedHashes[key] = f
	}

	if c.currentFiler != nil {
		if cur, ok := c.currentFiler.CurrentFile(f.Name); ok && f.IsDirectory() != cur.IsDirectory() {
			// We are attempting to replace a file with a directory or vice
			// versa. The existing item must be deleted before we can put the
			// new one in place. Queue that as well; the ordering will be
			// corrected later. If this is a replacement of a directory tree
			// with a file, we should have deletes for all the children as
			// well in this batch, and those will be taken care of before
			// this delete due to the ordering.
			cur.Blocks = nil
			cur.Flags |= protocol.FlagDeleted
			c.queue = append(c.queue, fileInfo{FileInfo: cur, synthetic: true})
		}
	}
}

// Apply performs the changes that are queued on the Changeset and returns
// nil on an error describing any problems encountered. If non-nil, the
// returned error is of the type ApplyError.
func (c *ChangeSet) Apply() error {
	c.mut.Lock()
	c.sortQueue()
	c.mut.Unlock()

	if shouldDebug() {
		c.mut.Lock()
		l.Debugf("Applying change set with %d items:", len(c.queue))
		for _, f := range c.queue {
			switch {
			case f.IsSymlink() && f.IsDeleted():
				l.Debugln("-l", f)
			case f.IsSymlink():
				l.Debugln("+l", f)
			case f.IsDirectory() && f.IsDeleted():
				l.Debugln("-d", f)
			case f.IsDirectory():
				l.Debugln("+d", f)
			case f.IsDeleted():
				l.Debugln("-f", f)
			default:
				l.Debugln("+f", f)
			}
		}
		c.mut.Unlock()
	}

	var errors []opError
	for {
		c.mut.Lock()
		if len(c.queue) == 0 {
			// We're done
			c.mut.Unlock()
			break
		}

		// Grab the frontmost item of the queue and shift the queue to the
		// next item.
		f := c.queue[0]
		c.queue = c.queue[1:]
		c.mut.Unlock()

		var err *opError

		// Ordering is relevant in this switch. A symlink to a directory
		// satisfies both IsSymlink() and IsDirectory() but must be treated
		// as a symlink.
		switch {
		case f.IsSymlink():
			if f.IsDeleted() {
				err = c.progressAccount(c.deleteSymlink, f)
			} else {
				err = c.progressAccount(c.writeSymlink, f)
			}

		case f.IsDirectory():
			if f.IsDeleted() {
				err = c.progressAccount(c.deleteDir, f)
			} else {
				err = c.progressAccount(c.writeDir, f)
			}

		default: // A file
			if f.IsDeleted() {
				err = c.progressAccount(c.deleteFile, f)
			} else {
				hash := string(f.Hash())
				if old, ok := c.deletedHashes[hash]; ok {
					// There exists an identical file that we are supposed to
					// delete. Lets instead rename it.
					delete(c.deletedHashes, hash)
					err = c.progressAccount(c.renameFileFrom(old), f)
				} else {
					err = c.progressAccount(c.writeFile, f)
				}
			}
		}

		if err != nil {
			errors = append(errors, *err)
		}
	}

	l.Debugln("Change set applied with", len(errors), "errors")
	if len(errors) > 0 {
		return applyError{errors}
	}

	return nil
}

func (c *ChangeSet) Shuffle() {
	c.mut.Lock()
	defer c.mut.Unlock()

	l := len(c.queue)
	for i := range c.queue {
		r := rand.Intn(l)
		c.queue[i], c.queue[r] = c.queue[r], c.queue[i]
	}
}

func (c *ChangeSet) SortSmallestFirst() {
	c.mut.Lock()
	defer c.mut.Unlock()

	sort.Sort(smallestFirst(c.queue))
}

func (c *ChangeSet) SortLargestFirst() {
	c.mut.Lock()
	defer c.mut.Unlock()

	sort.Sort(sort.Reverse(smallestFirst(c.queue)))
}

func (c *ChangeSet) SortOldestFirst() {
	c.mut.Lock()
	defer c.mut.Unlock()

	sort.Sort(oldestFirst(c.queue))
}

func (c *ChangeSet) SortNewestFirst() {
	c.mut.Lock()
	defer c.mut.Unlock()

	sort.Sort(sort.Reverse(oldestFirst(c.queue)))
}

// BringToFront moves the named item to the front of the queue. Note that
// Apply performs a dependency sort so the order may be overridden if
// BringToFront is called before Apply. Conversely, calling BringToFront
// after Apply will override the dependency order and can potentially result
// in a failure to apply the change set. It is safe to call BringToFront
// concurrently with Apply.
func (c *ChangeSet) BringToFront(name string) {
	c.mut.Lock()
	defer c.mut.Unlock()

	for i, v := range c.queue {
		if v.Name == name {
			// Move the first i elements one step to the right, overwriting
			// c.queue[i]
			copy(c.queue[1:], c.queue[:i])
			// Stash the element we found at c.queue[i] in c.queue[0]
			c.queue[0] = v
			return
		}
	}
}

// QueueNames returns the names of the items in the queue, in queue order
func (c *ChangeSet) QueueNames() []string {
	c.mut.Lock()
	defer c.mut.Unlock()

	names := make([]string, len(c.queue))
	for i, v := range c.queue {
		names[i] = v.Name
	}
	return names
}

func (c *ChangeSet) Size() int {
	c.mut.Lock()
	defer c.mut.Unlock()
	return len(c.queue)
}

// progressAccount calls fn(f) and returns it's return value, while ensuring
// that the Progresser is called appropriately before and after.
func (c *ChangeSet) progressAccount(fn func(protocol.FileInfo) *opError, f fileInfo) *opError {
	if f.synthetic {
		// This is a file operation we inserted into the queue ourselves to
		// satisfy dependencies.
		return fn(f.FileInfo)
	}

	c.progresser.Started(f.FileInfo)

	err := fn(f.FileInfo)
	if err != nil {
		c.progresser.Completed(f.FileInfo, err)
		return err
	}

	c.progresser.Completed(f.FileInfo, nil)

	return nil
}

// The usual sort.Interface boilerplate

type smallestFirst []fileInfo

func (s smallestFirst) Len() int           { return len(s) }
func (s smallestFirst) Less(a, b int) bool { return s[a].Size() < s[b].Size() }
func (s smallestFirst) Swap(a, b int)      { s[a], s[b] = s[b], s[a] }

type oldestFirst []fileInfo

func (s oldestFirst) Len() int           { return len(s) }
func (s oldestFirst) Less(a, b int) bool { return s[a].Modified < s[b].Modified }
func (s oldestFirst) Swap(a, b int)      { s[a], s[b] = s[b], s[a] }

// The error types

type opError struct {
	op         string
	file       string
	err        error
	mustRescan bool
}

func (e opError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.op, e.file, e.err)
}

type applyError struct {
	errors []opError
}

func (e applyError) Error() string {
	return fmt.Sprintf("applyError: %d errors", len(e.errors))
}

func (e applyError) MustRescan() bool {
	for _, err := range e.errors {
		if err.mustRescan {
			return true
		}
	}
	return false
}

func (e applyError) Errors() []error {
	errs := make([]error, len(e.errors))
	for i, err := range e.errors {
		errs[i] = err
	}
	return errs
}

// A no-op Progresser

type nilProgresser struct{}

func (nilProgresser) Started(file protocol.FileInfo) {}

func (nilProgresser) Progress(file protocol.FileInfo, copied, requested, downloaded int) {}

func (nilProgresser) Completed(file protocol.FileInfo, err error) {}
