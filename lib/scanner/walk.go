// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/rcrowley/go-metrics"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/text/unicode/norm"
)

var maskModePerm fs.FileMode

func init() {
	if runtime.GOOS == "windows" {
		// There is no user/group/others in Windows' read-only
		// attribute, and all "w" bits are set in fs.FileMode
		// if the file is not read-only.  Do not send these
		// group/others-writable bits to other devices in order to
		// avoid unexpected world-writable files on other platforms.
		maskModePerm = fs.ModePerm & 0755
	} else {
		maskModePerm = fs.ModePerm
	}
}

type Config struct {
	// Folder for which the walker has been created
	Folder string
	// Limit walking to these paths within Dir, or no limit if Sub is empty
	Subs []string
	// If Matcher is not nil, it is used to identify files to ignore which were specified by the user.
	Matcher *ignore.Matcher
	// Number of hours to keep temporary files for
	TempLifetime time.Duration
	// If CurrentFiler is not nil, it is queried for the current file before rescanning.
	CurrentFiler CurrentFiler
	// The Filesystem provides an abstraction on top of the actual filesystem.
	Filesystem fs.Filesystem
	// If IgnorePerms is true, changes to permission bits will not be
	// detected. Scanned files will get zero permission bits and the
	// NoPermissionBits flag set.
	IgnorePerms bool
	// When AutoNormalize is set, file names that are in UTF8 but incorrect
	// normalization form will be corrected.
	AutoNormalize bool
	// Number of routines to use for hashing
	Hashers int
	// Our vector clock id
	ShortID protocol.ShortID
	// Optional progress tick interval which defines how often FolderScanProgress
	// events are emitted. Negative number means disabled.
	ProgressTickIntervalS int
	// Whether to use large blocks for large files or the old standard of 128KiB for everything.
	UseLargeBlocks bool
}

type CurrentFiler interface {
	// CurrentFile returns the file as seen at last scan.
	CurrentFile(name string) (protocol.FileInfo, bool)
}

func Walk(ctx context.Context, cfg Config) chan protocol.FileInfo {
	w := walker{cfg}

	if w.CurrentFiler == nil {
		w.CurrentFiler = noCurrentFiler{}
	}
	if w.Filesystem == nil {
		panic("no filesystem specified")
	}
	if w.Matcher == nil {
		w.Matcher = ignore.New(w.Filesystem)
	}

	return w.walk(ctx)
}

type walker struct {
	Config
}

// Walk returns the list of files found in the local folder by scanning the
// file system. Files are blockwise hashed.
func (w *walker) walk(ctx context.Context) chan protocol.FileInfo {
	l.Debugln("Walk", w.Subs, w.Matcher)

	toHashChan := make(chan protocol.FileInfo)
	finishedChan := make(chan protocol.FileInfo)

	// A routine which walks the filesystem tree, and sends files which have
	// been modified to the counter routine.
	go func() {
		hashFiles := w.walkAndHashFiles(ctx, toHashChan, finishedChan)
		if len(w.Subs) == 0 {
			w.Filesystem.Walk(".", hashFiles)
		} else {
			for _, sub := range w.Subs {
				w.Filesystem.Walk(sub, hashFiles)
			}
		}
		close(toHashChan)
	}()

	// We're not required to emit scan progress events, just kick off hashers,
	// and feed inputs directly from the walker.
	if w.ProgressTickIntervalS < 0 {
		newParallelHasher(ctx, w.Filesystem, w.Hashers, finishedChan, toHashChan, nil, nil)
		return finishedChan
	}

	// Defaults to every 2 seconds.
	if w.ProgressTickIntervalS == 0 {
		w.ProgressTickIntervalS = 2
	}

	ticker := time.NewTicker(time.Duration(w.ProgressTickIntervalS) * time.Second)

	// We need to emit progress events, hence we create a routine which buffers
	// the list of files to be hashed, counts the total number of
	// bytes to hash, and once no more files need to be hashed (chan gets closed),
	// start a routine which periodically emits FolderScanProgress events,
	// until a stop signal is sent by the parallel hasher.
	// Parallel hasher is stopped by this routine when we close the channel over
	// which it receives the files we ask it to hash.
	go func() {
		var filesToHash []protocol.FileInfo
		var total int64 = 1

		for file := range toHashChan {
			filesToHash = append(filesToHash, file)
			total += file.Size
		}

		realToHashChan := make(chan protocol.FileInfo)
		done := make(chan struct{})
		progress := newByteCounter()

		newParallelHasher(ctx, w.Filesystem, w.Hashers, finishedChan, realToHashChan, progress, done)

		// A routine which actually emits the FolderScanProgress events
		// every w.ProgressTicker ticks, until the hasher routines terminate.
		go func() {
			defer progress.Close()

			for {
				select {
				case <-done:
					l.Debugln("Walk progress done", w.Folder, w.Subs, w.Matcher)
					ticker.Stop()
					return
				case <-ticker.C:
					current := progress.Total()
					rate := progress.Rate()
					l.Debugf("Walk %s %s current progress %d/%d at %.01f MiB/s (%d%%)", w.Folder, w.Subs, current, total, rate/1024/1024, current*100/total)
					events.Default.Log(events.FolderScanProgress, map[string]interface{}{
						"folder":  w.Folder,
						"current": current,
						"total":   total,
						"rate":    rate, // bytes per second
					})
				case <-ctx.Done():
					ticker.Stop()
					return
				}
			}
		}()

	loop:
		for _, file := range filesToHash {
			l.Debugln("real to hash:", file.Name)
			select {
			case realToHashChan <- file:
			case <-ctx.Done():
				break loop
			}
		}
		close(realToHashChan)
	}()

	return finishedChan
}

func (w *walker) walkAndHashFiles(ctx context.Context, fchan, dchan chan protocol.FileInfo) fs.WalkFunc {
	now := time.Now()
	ignoredParent := ""

	return func(path string, info fs.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Return value used when we are returning early and don't want to
		// process the item. For directories, this means do-not-descend.
		var skip error // nil
		// info nil when error is not nil
		if info != nil && info.IsDir() {
			skip = fs.SkipDir
		}

		if err != nil {
			l.Debugln("error:", path, info, err)
			return skip
		}

		if path == "." {
			return nil
		}

		if fs.IsTemporary(path) {
			l.Debugln("temporary:", path)
			if info.IsRegular() && info.ModTime().Add(w.TempLifetime).Before(now) {
				w.Filesystem.Remove(path)
				l.Debugln("removing temporary:", path, info.ModTime())
			}
			return nil
		}

		if fs.IsInternal(path) {
			l.Debugln("ignored (internal):", path)
			return skip
		}

		if !utf8.ValidString(path) {
			l.Warnf("File name %q is not in UTF8 encoding; skipping.", path)
			return skip
		}

		if w.Matcher.Match(path).IsIgnored() {
			l.Debugln("ignored (patterns):", path)
			// Only descend if matcher says so and the current file is not a symlink.
			if w.Matcher.SkipIgnoredDirs() || info.IsSymlink() {
				return skip
			}
			// If the parent wasn't ignored already, set this path as the "highest" ignored parent
			if info.IsDir() && (ignoredParent == "" || !strings.HasPrefix(path, ignoredParent+string(fs.PathSeparator))) {
				ignoredParent = path
			}
			return nil
		}

		if ignoredParent == "" {
			// parent isn't ignored, nothing special
			return w.handleItem(ctx, path, fchan, dchan, skip)
		}

		// Part of current path below the ignored (potential) parent
		rel := strings.TrimPrefix(path, ignoredParent+string(fs.PathSeparator))

		// ignored path isn't actually a parent of the current path
		if rel == path {
			ignoredParent = ""
			return w.handleItem(ctx, path, fchan, dchan, skip)
		}

		// The previously ignored parent directories of the current, not
		// ignored path need to be handled as well.
		if err = w.handleItem(ctx, ignoredParent, fchan, dchan, skip); err != nil {
			return err
		}
		for _, name := range strings.Split(rel, string(fs.PathSeparator)) {
			ignoredParent = filepath.Join(ignoredParent, name)
			if err = w.handleItem(ctx, ignoredParent, fchan, dchan, skip); err != nil {
				return err
			}
		}
		ignoredParent = ""

		return nil
	}
}

func (w *walker) handleItem(ctx context.Context, path string, fchan, dchan chan protocol.FileInfo, skip error) error {
	info, err := w.Filesystem.Lstat(path)
	// An error here would be weird as we've already gotten to this point, but act on it nonetheless
	if err != nil {
		return skip
	}

	path, shouldSkip := w.normalizePath(path, info)
	if shouldSkip {
		return skip
	}

	switch {
	case info.IsSymlink():
		if err := w.walkSymlink(ctx, path, dchan); err != nil {
			return err
		}
		if info.IsDir() {
			// under no circumstances shall we descend into a symlink
			return fs.SkipDir
		}
		return nil

	case info.IsDir():
		err = w.walkDir(ctx, path, info, dchan)

	case info.IsRegular():
		err = w.walkRegular(ctx, path, info, fchan)
	}

	return err
}

func (w *walker) walkRegular(ctx context.Context, relPath string, info fs.FileInfo, fchan chan protocol.FileInfo) error {
	curFile, hasCurFile := w.CurrentFiler.CurrentFile(relPath)

	newMode := uint32(info.Mode())
	if runtime.GOOS == "windows" {
		if osutil.IsWindowsExecutable(relPath) {
			// Set executable bits on files with executable extenions (.exe,
			// .bat, etc).
			newMode |= 0111
		} else if hasCurFile {
			// If we have an existing index entry, copy the executable bits
			// from there.
			newMode |= (curFile.Permissions & 0111)
		}
	}

	blockSize := protocol.MinBlockSize

	if w.UseLargeBlocks {
		blockSize = protocol.BlockSize(info.Size())

		if hasCurFile {
			// Check if we should retain current block size.
			curBlockSize := curFile.BlockSize()
			if blockSize > curBlockSize && blockSize/curBlockSize <= 2 {
				// New block size is larger, but not more than twice larger.
				// Retain.
				blockSize = curBlockSize
			} else if curBlockSize > blockSize && curBlockSize/blockSize <= 2 {
				// Old block size is larger, but not more than twice larger.
				// Retain.
				blockSize = curBlockSize
			}
		}
	}

	f := protocol.FileInfo{
		Name:          relPath,
		Type:          protocol.FileInfoTypeFile,
		Version:       curFile.Version.Update(w.ShortID),
		Permissions:   newMode & uint32(maskModePerm),
		NoPermissions: w.IgnorePerms,
		ModifiedS:     info.ModTime().Unix(),
		ModifiedNs:    int32(info.ModTime().Nanosecond()),
		ModifiedBy:    w.ShortID,
		Size:          info.Size(),
		RawBlockSize:  int32(blockSize),
	}

	if hasCurFile {
		if curFile.IsEquivalent(f, w.IgnorePerms, true) {
			return nil
		}
		if curFile.ShouldConflict() {
			// The old file was invalid for whatever reason and probably not
			// up to date with what was out there in the cluster. Drop all
			// others from the version vector to indicate that we haven't
			// taken their version into account, and possibly cause a
			// conflict.
			f.Version = f.Version.DropOthers(w.ShortID)
		}
		l.Debugln("rescan:", curFile, info.ModTime().Unix(), info.Mode()&fs.ModePerm)
	}

	l.Debugln("to hash:", relPath, f)

	select {
	case fchan <- f:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (w *walker) walkDir(ctx context.Context, relPath string, info fs.FileInfo, dchan chan protocol.FileInfo) error {
	cf, ok := w.CurrentFiler.CurrentFile(relPath)

	f := protocol.FileInfo{
		Name:          relPath,
		Type:          protocol.FileInfoTypeDirectory,
		Version:       cf.Version.Update(w.ShortID),
		Permissions:   uint32(info.Mode() & maskModePerm),
		NoPermissions: w.IgnorePerms,
		ModifiedS:     info.ModTime().Unix(),
		ModifiedNs:    int32(info.ModTime().Nanosecond()),
		ModifiedBy:    w.ShortID,
	}

	if ok {
		if cf.IsEquivalent(f, w.IgnorePerms, true) {
			return nil
		}
		if cf.ShouldConflict() {
			// The old file was invalid for whatever reason and probably not
			// up to date with what was out there in the cluster. Drop all
			// others from the version vector to indicate that we haven't
			// taken their version into account, and possibly cause a
			// conflict.
			f.Version = f.Version.DropOthers(w.ShortID)
		}
	}

	l.Debugln("dir:", relPath, f)

	select {
	case dchan <- f:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// walkSymlink returns nil or an error, if the error is of the nature that
// it should stop the entire walk.
func (w *walker) walkSymlink(ctx context.Context, relPath string, dchan chan protocol.FileInfo) error {
	// Symlinks are not supported on Windows. We ignore instead of returning
	// an error.
	if runtime.GOOS == "windows" {
		return nil
	}

	// We always rehash symlinks as they have no modtime or
	// permissions. We check if they point to the old target by
	// checking that their existing blocks match with the blocks in
	// the index.

	target, err := w.Filesystem.ReadSymlink(relPath)
	if err != nil {
		l.Debugln("readlink error:", relPath, err)
		return nil
	}

	cf, ok := w.CurrentFiler.CurrentFile(relPath)

	f := protocol.FileInfo{
		Name:          relPath,
		Type:          protocol.FileInfoTypeSymlink,
		Version:       cf.Version.Update(w.ShortID),
		NoPermissions: true, // Symlinks don't have permissions of their own
		SymlinkTarget: target,
		ModifiedBy:    w.ShortID,
	}

	if ok {
		if cf.IsEquivalent(f, w.IgnorePerms, true) {
			return nil
		}
		if cf.ShouldConflict() {
			// The old file was invalid for whatever reason and probably not
			// up to date with what was out there in the cluster. Drop all
			// others from the version vector to indicate that we haven't
			// taken their version into account, and possibly cause a
			// conflict.
			f.Version = f.Version.DropOthers(w.ShortID)
		}
	}

	l.Debugln("symlink changedb:", relPath, f)

	select {
	case dchan <- f:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// normalizePath returns the normalized relative path (possibly after fixing
// it on disk), or skip is true.
func (w *walker) normalizePath(path string, info fs.FileInfo) (normPath string, skip bool) {
	if runtime.GOOS == "darwin" {
		// Mac OS X file names should always be NFD normalized.
		normPath = norm.NFD.String(path)
	} else {
		// Every other OS in the known universe uses NFC or just plain
		// doesn't bother to define an encoding. In our case *we* do care,
		// so we enforce NFC regardless.
		normPath = norm.NFC.String(path)
	}

	if path == normPath {
		// The file name is already normalized: nothing to do
		return path, false
	}

	if !w.AutoNormalize {
		// We're not authorized to do anything about it, so complain and skip.

		l.Warnf("File name %q is not in the correct UTF8 normalization form; skipping.", path)
		return "", true
	}

	// We will attempt to normalize it.
	normInfo, err := w.Filesystem.Lstat(normPath)
	if fs.IsNotExist(err) {
		// Nothing exists with the normalized filename. Good.
		if err = w.Filesystem.Rename(path, normPath); err != nil {
			l.Infof(`Error normalizing UTF8 encoding of file "%s": %v`, path, err)
			return "", true
		}
		l.Infof(`Normalized UTF8 encoding of file name "%s".`, path)
	} else if w.Filesystem.SameFile(info, normInfo) {
		// With some filesystems (ZFS), if there is an un-normalized path and you ask whether the normalized
		// version exists, it responds with true. Therefore we need to check fs.SameFile as well.
		// In this case, a call to Rename won't do anything, so we have to rename via a temp file.

		// We don't want to use the standard syncthing prefix here, as that will result in the file being ignored
		// and eventually deleted by Syncthing if the rename back fails.

		tempPath := fs.TempNameWithPrefix(normPath, "")
		if err = w.Filesystem.Rename(path, tempPath); err != nil {
			l.Infof(`Error during normalizing UTF8 encoding of file "%s" (renamed to "%s"): %v`, path, tempPath, err)
			return "", true
		}
		if err = w.Filesystem.Rename(tempPath, normPath); err != nil {
			// I don't ever expect this to happen, but if it does, we should probably tell our caller that the normalized
			// path is the temp path: that way at least the user's data still gets synced.
			l.Warnf(`Error renaming "%s" to "%s" while normalizating UTF8 encoding: %v. You will want to rename this file back manually`, tempPath, normPath, err)
			return tempPath, false
		}
	} else {
		// There is something already in the way at the normalized
		// file name.
		l.Infof(`File "%s" path has UTF8 encoding conflict with another file; ignoring.`, path)
		return "", true
	}

	return normPath, false
}

// A byteCounter gets bytes added to it via Update() and then provides the
// Total() and one minute moving average Rate() in bytes per second.
type byteCounter struct {
	total int64
	metrics.EWMA
	stop chan struct{}
}

func newByteCounter() *byteCounter {
	c := &byteCounter{
		EWMA: metrics.NewEWMA1(), // a one minute exponentially weighted moving average
		stop: make(chan struct{}),
	}
	go c.ticker()
	return c
}

func (c *byteCounter) ticker() {
	// The metrics.EWMA expects clock ticks every five seconds in order to
	// decay the average properly.
	t := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-t.C:
			c.Tick()
		case <-c.stop:
			t.Stop()
			return
		}
	}
}

func (c *byteCounter) Update(bytes int64) {
	atomic.AddInt64(&c.total, bytes)
	c.EWMA.Update(bytes)
}

func (c *byteCounter) Total() int64 {
	return atomic.LoadInt64(&c.total)
}

func (c *byteCounter) Close() {
	close(c.stop)
}

// A no-op CurrentFiler

type noCurrentFiler struct{}

func (noCurrentFiler) CurrentFile(name string) (protocol.FileInfo, bool) {
	return protocol.FileInfo{}, false
}
