// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	metrics "github.com/rcrowley/go-metrics"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/text/unicode/norm"
)

type Config struct {
	// Folder for which the walker has been created
	Folder string
	// Limit walking to these paths within Dir, or no limit if Sub is empty
	Subs []string
	// If Matcher is not nil, it is used to identify files to ignore which were specified by the user.
	Matcher *ignore.Matcher
	// Number of hours to keep temporary files for
	TempLifetime time.Duration
	// Walks over file infos as present in the db before the scan alphabetically.
	Have HaveWalker
	// Returns the currently global item truncated
	Global TruncatedGlobaler
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
	// Local flags to set on scanned files
	LocalFlags uint32
	// Modification time is to be considered unchanged if the difference is lower.
	ModTimeWindow time.Duration
	// Event logger to which the scan progress events are sent
	EventLogger events.Logger
}

type HaveWalker interface {
	// Walk passes all local file infos from the db which start with prefix
	// to out and aborts early if ctx is cancelled.
	Walk(prefix string, ctx context.Context, out chan<- protocol.FileInfo)
}

type TruncatedGlobaler interface {
	GlobalTruncated(name string) (protocol.FileIntf, bool)
}

type fsWalkResult struct {
	path string
	info fs.FileInfo
	err  error
}

type ScanResult struct {
	New    protocol.FileInfo
	Old    protocol.FileInfo
	HasOld bool
	Err    error
	Path   string // to be set in case Err != nil
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

var (
	errUTF8Invalid       = errors.New("item is not in UTF8 encoding")
	errUTF8Normalization = errors.New("item is not in the correct UTF8 normalization form")
	errUTF8Conflict      = errors.New("item has UTF8 encoding conflict with another item")
)

type walker struct {
	Config
}

// Walk returns the list of files found in the local folder by scanning the
// file system. Files are blockwise hashed.
func (w *walker) walk(ctx context.Context) chan ScanResult {
	l.Debugln(w, "Walk", w.Subs, w.Matcher)

	haveChan := make(chan protocol.FileInfo)
	haveCtx, haveCancel := context.WithCancel(ctx)
	go w.dbWalkerRoutine(haveCtx, haveChan)

	fsChan := make(chan fsWalkResult)
	go w.fsWalkerRoutine(ctx, fsChan, haveCancel)

	toHashChan := make(chan ScanResult)
	finishedChan := make(chan ScanResult)
	go w.processWalkResults(ctx, fsChan, haveChan, toHashChan, finishedChan)

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
		var filesToHash []ScanResult
		var total int64 = 1

		for file := range toHashChan {
			filesToHash = append(filesToHash, file)
			total += file.New.Size
		}

		realToHashChan := make(chan ScanResult)
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
					l.Debugln(w, "Walk progress done", w.Folder, w.Subs, w.Matcher)
					ticker.Stop()
					return
				case <-ticker.C:
					current := progress.Total()
					rate := progress.Rate()
					l.Debugf("%s Walk %s s current progress %d/%d at %.01f MiB/s (%d%%)", w, w.Subs, current, total, rate/1024/1024, current*100/total)
					w.EventLogger.Log(events.FolderScanProgress, map[string]interface{}{
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
			l.Debugln(w, "real to hash:", file.New.Name)
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

// dbWalkerRoutine walks the db and sends back file infos to be compared to scan results.
func (w *walker) dbWalkerRoutine(ctx context.Context, haveChan chan<- protocol.FileInfo) {
	defer close(haveChan)
	defer l.Infoln("have done")

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
		if sub == "." {
			sub = ""
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
		if err := osutil.TraversesSymlink(w.Filesystem, filepath.Dir(sub)); err != nil {
			l.Debugf("%s Skip walking %v as it is below a symlink", w, sub)
			continue
		}
		if err := w.Filesystem.Walk(sub, walkFn); err != nil {
			haveCancel()
			break
		}
	}
}

func (w *walker) createFSWalkFn(ctx context.Context, fsChan chan<- fsWalkResult) fs.WalkFunc {
	now := time.Now()
	ignoredParent := ""

	return func(path string, info fs.FileInfo, err error) error {
		// Return value used when we are returning early and don't want to
		// process the item. For directories, this means do-not-descend.
		var skip error // nil
		// info nil when error is not nil
		if info != nil && info.IsDir() {
			skip = fs.SkipDir
		}

		if !utf8.ValidString(path) {
			fsWalkError(ctx, fsChan, path, errUTF8Invalid)
			return skip
		}

		if fs.IsTemporary(path) {
			l.Debugln(w, "temporary:", path, "err:", err)
			if err == nil && info.IsRegular() && info.ModTime().Add(w.TempLifetime).Before(now) {
				w.Filesystem.Remove(path)
				l.Debugln(w, "removing temporary:", path, info.ModTime())
			}
			return nil
		}

		if fs.IsInternal(path) {
			l.Debugln(w, "skip walking (internal):", path)
			return skip
		}

		if w.Matcher.Match(path).IsIgnored() {
			l.Debugln(w, "ignored (patterns):", path)
			// Only descend if matcher says so and the current file is not a symlink.
			if err != nil || w.Matcher.SkipIgnoredDirs() || info.IsSymlink() {
				return skip
			}
			// If the parent wasn't ignored already, set this path as the "highest" ignored parent
			if info.IsDir() && (ignoredParent == "" || !fs.IsParent(path, ignoredParent)) {
				ignoredParent = path
			}
			return nil
		}

		if err != nil {
			fsWalkError(ctx, fsChan, path, err)
			return skip
		}

		if path == "." {
			return nil
		}

		if ignoredParent == "" {
			// parent isn't ignored, nothing special
			return w.fsWalkSend(ctx, fsChan, path, info, skip)
		}

		// Part of current path below the ignored (potential) parent
		rel := strings.TrimPrefix(path, ignoredParent+string(fs.PathSeparator))

		// ignored path isn't actually a parent of the current path
		if rel == path {
			ignoredParent = ""
			return w.fsWalkSend(ctx, fsChan, path, info, skip)
		}

		// The previously ignored parent directories of the current, not
		// ignored path need to be handled as well.
		// Prepend an empty string to handle ignoredParent without anything
		// appended in the first iteration.
		for _, name := range append([]string{""}, strings.Split(rel, string(fs.PathSeparator))...) {
			ignoredParent = filepath.Join(ignoredParent, name)
			info, err = w.Filesystem.Lstat(ignoredParent)
			// An error here would be weird as we've already gotten to this point, but act on it nonetheless
			if err != nil {
				fsWalkError(ctx, fsChan, ignoredParent, err)
				return skip
			}
			if err = w.fsWalkSend(ctx, fsChan, ignoredParent, info, skip); err != nil {
				return err
			}
		}
		ignoredParent = ""

		return nil
	}
}

func (w *walker) fsWalkSend(ctx context.Context, fsChan chan<- fsWalkResult, path string, info fs.FileInfo, skip error) error {
	oldPath := path
	path, err := w.normalizePath(path, info)
	if err != nil {
		err = fmt.Errorf("normalizing path: %w", err)
		path = oldPath
	}

	select {
	case fsChan <- fsWalkResult{
		path: path,
		info: info,
		err:  err,
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// under no circumstances shall we descend into a symlink
	if info.IsSymlink() && info.IsDir() {
		l.Debugln(w, "skip walking (symlinked directory):", path)
		return skip
	}
	return nil
}

func fsWalkError(ctx context.Context, fsChan chan<- fsWalkResult, path string, err error) {
	select {
	case fsChan <- fsWalkResult{
		path: path,
		info: nil,
		err:  fmt.Errorf("scan: %w", err),
	}:
	case <-ctx.Done():
	}
}

func (w *walker) processWalkResults(ctx context.Context, fsChan <-chan fsWalkResult, haveChan <-chan protocol.FileInfo, toHashChan, finishedChan chan<- ScanResult) {
	ctxChan := ctx.Done()
	fsRes, fsChanOpen := <-fsChan
	currDBFile, haveChanOpen := <-haveChan
	for fsChanOpen {
		l.Infoln("A", currDBFile.Name, fsRes.path)
		if haveChanOpen {
			l.Infoln("B")
			// File infos below an error walking the filesystem tree
			// may be marked as ignored but should not be deleted.
			if fsRes.err != nil && (strings.HasPrefix(currDBFile.Name, fsRes.path+string(fs.PathSeparator)) || fsRes.path == ".") {
				l.Debugln(w, "error in filesystem on parent of existing item", currDBFile.Name)
				w.checkAndSetIgnored(currDBFile, finishedChan, ctxChan)
				currDBFile, haveChanOpen = <-haveChan
				continue
			}
			// Delete file infos that were not encountered when
			// walking the filesystem tree, except on error (see
			// above) or if they are ignored.
			if currDBFile.Name < fsRes.path {
				l.Debugln(w, "detected deleted", currDBFile.Name)
				w.checkIgnoredAndDelete(currDBFile, finishedChan, ctxChan)
				currDBFile, haveChanOpen = <-haveChan
				continue
			}
		}

		var oldFile protocol.FileInfo
		var hasOldFile bool
		if haveChanOpen && currDBFile.Name == fsRes.path {
			oldFile = currDBFile
			hasOldFile = true
			currDBFile, haveChanOpen = <-haveChan
		}

		if fsRes.err != nil {
			if errors.Is(fsRes.err, fs.ErrNotExist) && !oldFile.IsEmpty() && !oldFile.Deleted {
				nf := oldFile.DeletedCopy(w.ShortID)
				nf.LocalFlags = w.LocalFlags
				select {
				case finishedChan <- ScanResult{
					New: nf,
					Old: oldFile,
				}:
				case <-ctx.Done():
					return
				}
			}
			if !errors.Is(fsRes.err, fs.ErrNotExist) {
				select {
				case finishedChan <- ScanResult{
					Err:  fsRes.err,
					Path: fsRes.path,
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
			w.walkDir(ctx, fsRes.path, fsRes.info, oldFile, hasOldFile, finishedChan)

		case fsRes.info.IsSymlink():
			w.walkSymlink(ctx, fsRes.path, fsRes.info, oldFile, hasOldFile, finishedChan)

		case fsRes.info.IsRegular():
			w.walkRegular(ctx, fsRes.path, fsRes.info, oldFile, hasOldFile, toHashChan)
		}

		fsRes, fsChanOpen = <-fsChan
	}

	// Filesystem tree walking finished, if there is anything left in the
	// db, mark it as deleted, except when it's ignored.
	if haveChanOpen {
		l.Infoln("C", currDBFile.Name)
		w.checkIgnoredAndDelete(currDBFile, finishedChan, ctxChan)
		for currDBFile = range haveChan {
			l.Infoln("D", currDBFile.Name)
			w.checkIgnoredAndDelete(currDBFile, finishedChan, ctxChan)
		}
	}

	close(toHashChan)
}

func (w *walker) checkIgnoredAndDelete(f protocol.FileInfo, finishedChan chan<- ScanResult, done <-chan struct{}) {
	if w.checkAndSetIgnored(f, finishedChan, done) {
		return
	}

	// Check if global is deleted too and if yes, drop local flag.
	if f.Deleted && f.IsReceiveOnlyChanged() {
		if global, _ := w.Global.GlobalTruncated(f.Name); !global.IsDeleted() {
			return
		}
		nf := f.DeletedCopy(w.ShortID) // Is already deleted, still want to copy
		nf.Version = protocol.Vector{}
		nf.LocalFlags = 0
		select {
		case finishedChan <- ScanResult{
			New: nf,
			Old: f,
		}:
		case <-done:
		}
		return
	}

	if f.Deleted || f.IsUnsupported() {
		return
	}

	nf := f.DeletedCopy(w.ShortID)
	nf.LocalFlags = w.LocalFlags
	if f.ShouldConflict() {
		nf.Version = protocol.Vector{}
	}
	select {
	case finishedChan <- ScanResult{
		New: nf,
		Old: f,
	}:
	case <-done:
	}
}

func (w *walker) checkAndSetIgnored(f protocol.FileInfo, finishedChan chan<- ScanResult, done <-chan struct{}) bool {
	if !w.Matcher.Match(f.Name).IsIgnored() {
		return false
	}

	if !f.IsIgnored() {
		select {
		case finishedChan <- ScanResult{
			New: f.IgnoredCopy(w.ShortID),
			Old: f,
		}:
		case <-done:
		}
	}

	return true
}

func (w *walker) walkRegular(ctx context.Context, relPath string, info fs.FileInfo, curFile protocol.FileInfo, hasCurFile bool, toHashChan chan<- ScanResult) {
	blockSize := protocol.BlockSize(info.Size())

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

	f, _ := CreateFileInfo(info, relPath, nil)
	f = w.updateFileInfo(f, curFile)
	f.NoPermissions = w.IgnorePerms
	f.RawBlockSize = int32(blockSize)

	if hasCurFile {
		if curFile.IsEquivalentOptional(f, w.ModTimeWindow, w.IgnorePerms, true, w.LocalFlags) {
			return
		}
		if curFile.ShouldConflict() {
			// The old file was invalid for whatever reason and probably not
			// up to date with what was out there in the cluster. Drop all
			// others from the version vector to indicate that we haven't
			// taken their version into account, and possibly cause a
			// conflict.
			f.Version = f.Version.DropOthers(w.ShortID)
		}
		l.Debugln(w, "rescan:", curFile, info.ModTime().Unix(), info.Mode()&fs.ModePerm)
	}

	l.Debugln(w, "to hash:", relPath, f)

	select {
	case toHashChan <- ScanResult{
		New:    f,
		Old:    curFile,
		HasOld: hasCurFile,
	}:
	case <-ctx.Done():
	}
}

func (w *walker) walkDir(ctx context.Context, relPath string, info fs.FileInfo, curFile protocol.FileInfo, hasCurFile bool, finishedChan chan<- ScanResult) {
	f, _ := CreateFileInfo(info, relPath, nil)
	f = w.updateFileInfo(f, curFile)
	f.NoPermissions = w.IgnorePerms

	if hasCurFile {
		if curFile.IsEquivalentOptional(f, w.ModTimeWindow, w.IgnorePerms, true, w.LocalFlags) {
			return
		}
		if curFile.ShouldConflict() {
			// The old file was invalid for whatever reason and probably not
			// up to date with what was out there in the cluster. Drop all
			// others from the version vector to indicate that we haven't
			// taken their version into account, and possibly cause a
			// conflict.
			f.Version = f.Version.DropOthers(w.ShortID)
		}
	}

	l.Debugln(w, "dir:", relPath, f)

	select {
	case finishedChan <- ScanResult{
		New:    f,
		Old:    curFile,
		HasOld: hasCurFile,
	}:
	case <-ctx.Done():
	}
}

// walkSymlink returns nil or an error, if the error is of the nature that
// it should stop the entire walk.
func (w *walker) walkSymlink(ctx context.Context, relPath string, info fs.FileInfo, curFile protocol.FileInfo, hasCurFile bool, finishedChan chan<- ScanResult) {
	// Symlinks are not supported on Windows. We ignore instead of returning
	// an error.
	if runtime.GOOS == "windows" {
		return
	}

	f, err := CreateFileInfo(info, relPath, w.Filesystem)
	if err != nil {
		select {
		case finishedChan <- ScanResult{
			Err:  fmt.Errorf("reading link: %w", err),
			Path: relPath,
		}:
		case <-ctx.Done():
			return
		}
		return
	}

	f = w.updateFileInfo(f, curFile)

	if hasCurFile {
		if curFile.IsEquivalentOptional(f, w.ModTimeWindow, w.IgnorePerms, true, w.LocalFlags) {
			return
		}
		if curFile.ShouldConflict() {
			// The old file was invalid for whatever reason and probably not
			// up to date with what was out there in the cluster. Drop all
			// others from the version vector to indicate that we haven't
			// taken their version into account, and possibly cause a
			// conflict.
			f.Version = f.Version.DropOthers(w.ShortID)
		}
	}

	l.Debugln(w, "symlink changedb:", relPath, f)

	select {
	case finishedChan <- ScanResult{
		New:    f,
		Old:    curFile,
		HasOld: hasCurFile,
	}:
	case <-ctx.Done():
	}
}

// normalizePath returns the normalized relative path (possibly after fixing
// it on disk), or skip is true.
func (w *walker) normalizePath(path string, info fs.FileInfo) (normPath string, err error) {
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
		return path, nil
	}

	if !w.AutoNormalize {
		// We're not authorized to do anything about it, so complain and skip.

		return "", errUTF8Normalization
	}

	// We will attempt to normalize it.
	normInfo, err := w.Filesystem.Lstat(normPath)
	if fs.IsNotExist(err) {
		// Nothing exists with the normalized filename. Good.
		if err = w.Filesystem.Rename(path, normPath); err != nil {
			return "", err
		}
		l.Infof(`Normalized UTF8 encoding of file name "%s".`, path)
		return normPath, nil
	}
	if w.Filesystem.SameFile(info, normInfo) {
		// With some filesystems (ZFS), if there is an un-normalized path and you ask whether the normalized
		// version exists, it responds with true. Therefore we need to check fs.SameFile as well.
		// In this case, a call to Rename won't do anything, so we have to rename via a temp file.

		// We don't want to use the standard syncthing prefix here, as that will result in the file being ignored
		// and eventually deleted by Syncthing if the rename back fails.

		tempPath := fs.TempNameWithPrefix(normPath, "")
		if err = w.Filesystem.Rename(path, tempPath); err != nil {
			return "", err
		}
		if err = w.Filesystem.Rename(tempPath, normPath); err != nil {
			// I don't ever expect this to happen, but if it does, we should probably tell our caller that the normalized
			// path is the temp path: that way at least the user's data still gets synced.
			l.Warnf(`Error renaming "%s" to "%s" while normalizating UTF8 encoding: %v. You will want to rename this file back manually`, tempPath, normPath, err)
			return tempPath, nil
		}
		return normPath, nil
	}
	// There is something already in the way at the normalized
	// file name.
	return "", errUTF8Conflict
}

// updateFileInfo updates walker specific members of protocol.FileInfo that do not depend on type
func (w *walker) updateFileInfo(file, curFile protocol.FileInfo) protocol.FileInfo {
	if file.Type == protocol.FileInfoTypeFile && runtime.GOOS == "windows" {
		// If we have an existing index entry, copy the executable bits
		// from there.
		file.Permissions |= (curFile.Permissions & 0111)
	}
	file.Version = curFile.Version.Update(w.ShortID)
	file.ModifiedBy = w.ShortID
	file.LocalFlags = w.LocalFlags
	return file
}

func (w *walker) String() string {
	return fmt.Sprintf("walker/%s@%p", w.Folder, w)
}

// A byteCounter gets bytes added to it via Update() and then provides the
// Total() and one minute moving average Rate() in bytes per second.
type byteCounter struct {
	total int64 // atomic, must remain 64-bit aligned
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

func CreateFileInfo(fi fs.FileInfo, name string, filesystem fs.Filesystem) (protocol.FileInfo, error) {
	f := protocol.FileInfo{Name: name}
	if fi.IsSymlink() {
		f.Type = protocol.FileInfoTypeSymlink
		target, err := filesystem.ReadSymlink(name)
		if err != nil {
			return protocol.FileInfo{}, err
		}
		f.SymlinkTarget = target
		f.NoPermissions = true // Symlinks don't have permissions of their own
		return f, nil
	}
	f.Permissions = uint32(fi.Mode() & fs.ModePerm)
	f.ModifiedS = fi.ModTime().Unix()
	f.ModifiedNs = int32(fi.ModTime().Nanosecond())
	if fi.IsDir() {
		f.Type = protocol.FileInfoTypeDirectory
		return f, nil
	}
	f.Size = fi.Size()
	f.Type = protocol.FileInfoTypeFile
	return f, nil
}
