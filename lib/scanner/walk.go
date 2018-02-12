// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"context"
	"errors"
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
	// BlockSize controls the size of the block used when hashing.
	BlockSize int
	// If Matcher is not nil, it is used to identify files to ignore which were specified by the user.
	Matcher *ignore.Matcher
	// Number of hours to keep temporary files for
	TempLifetime time.Duration
	// Walks over file infos as present in the db before the scan alphabetically.
	Have haveWalker
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
	// Whether or not we should also compute weak hashes
	UseWeakHashes bool
}

type haveWalker interface {
	// Walk passes all local file infos from the db which start with prefix
	// to out and aborts early if ctx is cancelled.
	Walk(prefix string, ctx context.Context, out chan<- protocol.FileInfo)
}

type fsWalkResult struct {
	path string
	info fs.FileInfo
	err  error
}

type ScanResult struct {
	New protocol.FileInfo
	Old protocol.FileInfo
}

func Walk(ctx context.Context, cfg Config) chan ScanResult {
	w := walker{cfg}

	if w.Have == nil {
		w.Have = noHaveWalker{}
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
func (w *walker) walk(ctx context.Context) chan ScanResult {
	l.Debugln("Walk", w.Subs, w.BlockSize, w.Matcher)

	haveChan := make(chan protocol.FileInfo)
	haveCtx, haveCancel := context.WithCancel(ctx)
	go w.dbWalkerRoutine(haveCtx, haveChan)

	fsChan := make(chan fsWalkResult)
	go w.fsWalkerRoutine(ctx, fsChan, haveCancel)

	toHashChan := make(chan ScanResult)
	finisherChan := make(chan ScanResult)
	go w.processWalkResults(ctx, fsChan, haveChan, toHashChan, finisherChan)

	outChan := make(chan ScanResult)
	go w.finisher(ctx, finisherChan, outChan)

	// We're not required to emit scan progress events, just kick off hashers,
	// and feed inputs directly from the walker.
	if w.ProgressTickIntervalS < 0 {
		newParallelHasher(ctx, w.Filesystem, w.BlockSize, w.Hashers, finisherChan, toHashChan, nil, nil, w.UseWeakHashes)
		return outChan
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
		var filesToHash []ScanResult
		var total int64 = 1

		for file := range toHashChan {
			filesToHash = append(filesToHash, file)
			total += file.New.Size
		}

		realToHashChan := make(chan ScanResult)
		done := make(chan struct{})
		progress := newByteCounter()

		newParallelHasher(ctx, w.Filesystem, w.BlockSize, w.Hashers, finisherChan, realToHashChan, progress, done, w.UseWeakHashes)

		// A routine which actually emits the FolderScanProgress events
		// every w.ProgressTicker ticks, until the hasher routines terminate.
		go func() {
			defer progress.Close()

			for {
				select {
				case <-done:
					l.Debugln("Walk progress done", w.Folder, w.Subs, w.BlockSize, w.Matcher)
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
			l.Debugln("real to hash:", file.New.Name)
			select {
			case realToHashChan <- file:
			case <-ctx.Done():
				break loop
			}
		}
		close(realToHashChan)
	}()

	return outChan
}

// dbWalkerRoutine walks the db and sends back file infos to be compared to scan results.
func (w *walker) dbWalkerRoutine(ctx context.Context, haveChan chan<- protocol.FileInfo) {
	defer close(haveChan)

	if len(w.Subs) == 0 {
		w.Have.Walk("", ctx, haveChan)
		return
	}

	for _, sub := range w.Subs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.Have.Walk(sub, ctx, haveChan)
	}
}

// fsWalkerRoutine walks the filesystem tree and sends back file infos and potential
// errors at paths that need to be processed.
func (w *walker) fsWalkerRoutine(ctx context.Context, fsChan chan<- fsWalkResult, haveCancel context.CancelFunc) {
	defer close(fsChan)

	walkFn := w.createFSWalkFn(ctx, fsChan)
	if len(w.Subs) == 0 {
		if err := w.Filesystem.Walk(".", walkFn); err != nil {
			haveCancel()
		}
		return
	}

	for _, sub := range w.Subs {
		if err := w.Filesystem.Walk(sub, walkFn); err != nil {
			haveCancel()
			break
		}
	}
}

func (w *walker) createFSWalkFn(ctx context.Context, fsChan chan<- fsWalkResult) fs.WalkFunc {
	now := time.Now()
	return func(path string, info fs.FileInfo, err error) error {
		// Return value used when we are returning early and don't want to
		// process the item. For directories, this means do-not-descend.
		var skip error // nil
		// info nil when error is not nil
		if info != nil && info.IsDir() {
			skip = fs.SkipDir
		}

		if path == "." {
			if err != nil {
				fsWalkError(ctx, fsChan, path, err)
				return skip
			}
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
			l.Debugln("skip walking (internal):", path)
			return skip
		}

		if w.Matcher.Match(path).IsIgnored() {
			l.Debugln("skip walking (patterns):", path)
			return skip
		}

		if err != nil {
			if sendErr := fsWalkError(ctx, fsChan, path, err); sendErr != nil {
				return sendErr
			}
			return skip
		}

		if !utf8.ValidString(path) {
			if err := fsWalkError(ctx, fsChan, path, errors.New("path isn't a valid utf8 string")); err != nil {
				return err
			}
			l.Warnf("File name %q is not in UTF8 encoding; skipping.", path)
			return skip
		}

		path, shouldSkip := w.normalizePath(path, info)
		if shouldSkip {
			if err := fsWalkError(ctx, fsChan, path, errors.New("failed to normalize path")); err != nil {
				return err
			}
			return skip
		}

		select {
		case fsChan <- fsWalkResult{
			path: path,
			info: info,
			err:  nil,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}

		// under no circumstances shall we descend into a symlink
		if info.IsSymlink() && info.IsDir() {
			l.Debugln("skip walking (symlinked directory):", path)
			return skip
		}

		return err
	}
}

func fsWalkError(ctx context.Context, dst chan<- fsWalkResult, path string, err error) error {
	select {
	case dst <- fsWalkResult{
		path: path,
		info: nil,
		err:  err,
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (w *walker) processWalkResults(ctx context.Context, fsChan <-chan fsWalkResult, haveChan <-chan protocol.FileInfo, toHashChan, finisherChan chan<- ScanResult) {
	ctxChan := ctx.Done()
	fsRes, fsChanOpen := <-fsChan
	currDBFile, haveChanOpen := <-haveChan
	for fsChanOpen {
		if haveChanOpen {
			// File infos below an error walking the filesystem tree
			// may be marked as ignored but should not be deleted.
			if fsRes.err != nil && (strings.HasPrefix(currDBFile.Name, fsRes.path+string(fs.PathSeparator)) || fsRes.path == ".") {
				w.checkIgnoredAndInvalidate(currDBFile, finisherChan, ctxChan)
				currDBFile, haveChanOpen = <-haveChan
				continue
			}
			// Delete file infos that were not encountered when
			// walking the filesystem tree, except on error (see
			// above) or if they are ignored.
			if currDBFile.Name < fsRes.path {
				w.checkIgnoredAndDelete(currDBFile, finisherChan, ctxChan)
				currDBFile, haveChanOpen = <-haveChan
				continue
			}
		}

		var oldFile protocol.FileInfo
		if haveChanOpen && currDBFile.Name == fsRes.path {
			oldFile = currDBFile
			currDBFile, haveChanOpen = <-haveChan
		}

		if fsRes.err != nil {
			if fs.IsNotExist(fsRes.err) && !oldFile.IsEmpty() && !oldFile.Deleted {
				select {
				case finisherChan <- ScanResult{
					New: oldFile.DeletedCopy(w.ShortID),
					Old: oldFile,
				}:
				case <-ctx.Done():
					return
				}
			}
			fsRes, fsChanOpen = <-fsChan
			continue
		}

		switch {
		case fsRes.info.IsDir():
			w.walkDir(ctx, fsRes.path, fsRes.info, oldFile, finisherChan)

		case fsRes.info.IsSymlink():
			w.walkSymlink(ctx, fsRes.path, oldFile, finisherChan)

		case fsRes.info.IsRegular():
			w.walkRegular(ctx, fsRes.path, fsRes.info, oldFile, toHashChan)
		}

		fsRes, fsChanOpen = <-fsChan
	}

	// Filesystem tree walking finished, if there is anything left in the
	// db, mark it as deleted, except when it's ignored.
	if haveChanOpen {
		w.checkIgnoredAndDelete(currDBFile, finisherChan, ctxChan)
		for currDBFile = range haveChan {
			w.checkIgnoredAndDelete(currDBFile, finisherChan, ctxChan)
		}
	}

	close(toHashChan)
}

func (w *walker) checkIgnoredAndDelete(f protocol.FileInfo, finisherChan chan<- ScanResult, done <-chan struct{}) {
	if w.checkIgnoredAndInvalidate(f, finisherChan, done) {
		return
	}

	if !f.Deleted {
		select {
		case finisherChan <- ScanResult{
			New: f.DeletedCopy(w.ShortID),
			Old: f,
		}:
		case <-done:
		}
	}
}

func (w *walker) checkIgnoredAndInvalidate(f protocol.FileInfo, finisherChan chan<- ScanResult, done <-chan struct{}) bool {
	if !w.Matcher.Match(f.Name).IsIgnored() {
		return false
	}

	if !f.Invalid {
		select {
		case finisherChan <- ScanResult{
			New: f.InvalidatedCopy(w.ShortID),
			Old: f,
		}:
		case <-done:
		}
	}

	return true
}

func (w *walker) walkRegular(ctx context.Context, relPath string, info fs.FileInfo, cf protocol.FileInfo, toHashChan chan<- ScanResult) {
	curMode := uint32(info.Mode())
	if runtime.GOOS == "windows" && osutil.IsWindowsExecutable(relPath) {
		curMode |= 0111
	}

	nf := protocol.FileInfo{
		Name:          relPath,
		Type:          protocol.FileInfoTypeFile,
		Version:       cf.Version.Update(w.ShortID),
		Permissions:   curMode & uint32(maskModePerm),
		NoPermissions: w.IgnorePerms,
		ModifiedS:     info.ModTime().Unix(),
		ModifiedNs:    int32(info.ModTime().Nanosecond()),
		ModifiedBy:    w.ShortID,
		Size:          info.Size(),
	}

	if nf.IsEquivalent(cf, w.IgnorePerms, true) {
		return
	}

	f := ScanResult{
		New: nf,
		Old: cf,
	}

	l.Debugln("to hash:", relPath, f)

	select {
	case toHashChan <- f:
	case <-ctx.Done():
	}
}

func (w *walker) walkDir(ctx context.Context, relPath string, info fs.FileInfo, cf protocol.FileInfo, finisherChan chan<- ScanResult) {
	nf := protocol.FileInfo{
		Name:          relPath,
		Type:          protocol.FileInfoTypeDirectory,
		Version:       cf.Version.Update(w.ShortID),
		Permissions:   uint32(info.Mode() & maskModePerm),
		NoPermissions: w.IgnorePerms,
		ModifiedS:     info.ModTime().Unix(),
		ModifiedNs:    int32(info.ModTime().Nanosecond()),
		ModifiedBy:    w.ShortID,
	}

	if nf.IsEquivalent(cf, w.IgnorePerms, true) {
		return
	}

	f := ScanResult{
		New: nf,
		Old: cf,
	}

	l.Debugln("dir:", relPath, f)

	select {
	case finisherChan <- f:
	case <-ctx.Done():
	}
}

// walkSymlink returns nil or an error, if the error is of the nature that
// it should stop the entire walk.
func (w *walker) walkSymlink(ctx context.Context, relPath string, cf protocol.FileInfo, finisherChan chan<- ScanResult) {
	// Symlinks are not supported on Windows. We ignore instead of returning
	// an error.
	if runtime.GOOS == "windows" {
		return
	}

	// We always rehash symlinks as they have no modtime or
	// permissions. We check if they point to the old target by
	// checking that their existing blocks match with the blocks in
	// the index.

	target, err := w.Filesystem.ReadSymlink(relPath)
	if err != nil {
		l.Debugln("readlink error:", relPath, err)
		return
	}

	nf := protocol.FileInfo{
		Name:          relPath,
		Type:          protocol.FileInfoTypeSymlink,
		Version:       cf.Version.Update(w.ShortID),
		NoPermissions: true, // Symlinks don't have permissions of their own
		SymlinkTarget: target,
	}

	if nf.IsEquivalent(cf, w.IgnorePerms, true) {
		return
	}

	f := ScanResult{
		New: nf,
		Old: cf,
	}

	l.Debugln("symlink changedb:", relPath, f)

	select {
	case finisherChan <- f:
	case <-ctx.Done():
	}
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

func (w *walker) finisher(ctx context.Context, finisherChan <-chan ScanResult, outChan chan<- ScanResult) {
	for r := range finisherChan {
		if !r.Old.IsEmpty() && r.Old.Invalid {
			// We do not want to override the global version with the file we
			// currently have. Keeping only our local counter makes sure we are in
			// conflict with any other existing versions, which will be resolved by
			// the normal pulling mechanisms.
			for i, c := range r.New.Version.Counters {
				if c.ID == w.ShortID {
					r.New.Version.Counters = r.New.Version.Counters[i : i+1]
					break
				}
			}
		}
		select {
		case outChan <- r:
		case <-ctx.Done():
			return
		}
	}

	close(outChan)
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

type noHaveWalker struct{}

func (noHaveWalker) Walk(prefix string, ctx context.Context, out chan<- protocol.FileInfo) {}
