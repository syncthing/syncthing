// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/ignore"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/symlinks"
	"golang.org/x/text/unicode/norm"
)

var maskModePerm os.FileMode

func init() {
	if runtime.GOOS == "windows" {
		// There is no user/group/others in Windows' read-only
		// attribute, and all "w" bits are set in os.FileInfo
		// if the file is not read-only.  Do not send these
		// group/others-writable bits to other devices in order to
		// avoid unexpected world-writable files on other platforms.
		maskModePerm = os.ModePerm & 0755
	} else {
		maskModePerm = os.ModePerm
	}
}

type Walker struct {
	// Dir is the base directory for the walk
	Dir string
	// Limit walking to these paths within Dir, or no limit if Sub is empty
	Subs []string
	// BlockSize controls the size of the block used when hashing.
	BlockSize int
	// If Matcher is not nil, it is used to identify files to ignore which were specified by the user.
	Matcher *ignore.Matcher
	// If TempNamer is not nil, it is used to ignore temporary files when walking.
	TempNamer TempNamer
	// Number of hours to keep temporary files for
	TempLifetime time.Duration
	// If CurrentFiler is not nil, it is queried for the current file before rescanning.
	CurrentFiler CurrentFiler
	// If MtimeRepo is not nil, it is used to provide mtimes on systems that don't support setting arbirtary mtimes.
	MtimeRepo *db.VirtualMtimeRepo
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
	ShortID uint64
}

type TempNamer interface {
	// Temporary returns a temporary name for the filed referred to by filepath.
	TempName(path string) string
	// IsTemporary returns true if path refers to the name of temporary file.
	IsTemporary(path string) bool
}

type CurrentFiler interface {
	// CurrentFile returns the file as seen at last scan.
	CurrentFile(name string) (protocol.FileInfo, bool)
}

// Walk returns the list of files found in the local folder by scanning the
// file system. Files are blockwise hashed.
func (w *Walker) Walk() (chan protocol.FileInfo, error) {
	if debug {
		l.Debugln("Walk", w.Dir, w.Subs, w.BlockSize, w.Matcher)
	}

	err := checkDir(w.Dir)
	if err != nil {
		return nil, err
	}

	files := make(chan protocol.FileInfo)
	hashedFiles := make(chan protocol.FileInfo)
	newParallelHasher(w.Dir, w.BlockSize, w.Hashers, hashedFiles, files)

	go func() {
		hashFiles := w.walkAndHashFiles(files)
		if len(w.Subs) == 0 {
			filepath.Walk(w.Dir, hashFiles)
		} else {
			for _, sub := range w.Subs {
				filepath.Walk(filepath.Join(w.Dir, sub), hashFiles)
			}
		}
		close(files)
	}()

	return hashedFiles, nil
}

func (w *Walker) walkAndHashFiles(fchan chan protocol.FileInfo) filepath.WalkFunc {
	now := time.Now()
	return func(p string, info os.FileInfo, err error) error {
		// Return value used when we are returning early and don't want to
		// process the item. For directories, this means do-not-descend.
		var skip error // nil
		// info nil when error is not nil
		if info != nil && info.IsDir() {
			skip = filepath.SkipDir
		}

		if err != nil {
			if debug {
				l.Debugln("error:", p, info, err)
			}
			return skip
		}

		rn, err := filepath.Rel(w.Dir, p)
		if err != nil {
			if debug {
				l.Debugln("rel error:", p, err)
			}
			return skip
		}

		if rn == "." {
			return nil
		}

		mtime := info.ModTime()
		if w.MtimeRepo != nil {
			mtime = w.MtimeRepo.GetMtime(rn, mtime)
		}

		if w.TempNamer != nil && w.TempNamer.IsTemporary(rn) {
			// A temporary file
			if debug {
				l.Debugln("temporary:", rn)
			}
			if info.Mode().IsRegular() && mtime.Add(w.TempLifetime).Before(now) {
				os.Remove(p)
				if debug {
					l.Debugln("removing temporary:", rn, mtime)
				}
			}
			return nil
		}

		if sn := filepath.Base(rn); sn == ".stignore" || sn == ".stfolder" ||
			strings.HasPrefix(rn, ".stversions") || w.Matcher.Match(rn) {
			// An ignored file
			if debug {
				l.Debugln("ignored:", rn)
			}
			return skip
		}

		if !utf8.ValidString(rn) {
			l.Warnf("File name %q is not in UTF8 encoding; skipping.", rn)
			return skip
		}

		var normalizedRn string
		if runtime.GOOS == "darwin" {
			// Mac OS X file names should always be NFD normalized.
			normalizedRn = norm.NFD.String(rn)
		} else {
			// Every other OS in the known universe uses NFC or just plain
			// doesn't bother to define an encoding. In our case *we* do care,
			// so we enforce NFC regardless.
			normalizedRn = norm.NFC.String(rn)
		}

		if rn != normalizedRn {
			// The file name was not normalized.

			if !w.AutoNormalize {
				// We're not authorized to do anything about it, so complain and skip.

				l.Warnf("File name %q is not in the correct UTF8 normalization form; skipping.", rn)
				return skip
			}

			// We will attempt to normalize it.
			normalizedPath := filepath.Join(w.Dir, normalizedRn)
			if _, err := osutil.Lstat(normalizedPath); os.IsNotExist(err) {
				// Nothing exists with the normalized filename. Good.
				if err = os.Rename(p, normalizedPath); err != nil {
					l.Infof(`Error normalizing UTF8 encoding of file "%s": %v`, rn, err)
					return skip
				}
				l.Infof(`Normalized UTF8 encoding of file name "%s".`, rn)
			} else {
				// There is something already in the way at the normalized
				// file name.
				l.Infof(`File "%s" has UTF8 encoding conflict with another file; ignoring.`, rn)
				return skip
			}

			rn = normalizedRn
		}

		var cf protocol.FileInfo
		var ok bool

		// Index wise symlinks are always files, regardless of what the target
		// is, because symlinks carry their target path as their content.
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			// If the target is a directory, do NOT descend down there. This
			// will cause files to get tracked, and removing the symlink will
			// as a result remove files in their real location.
			if !symlinks.Supported {
				return skip
			}

			// We always rehash symlinks as they have no modtime or
			// permissions. We check if they point to the old target by
			// checking that their existing blocks match with the blocks in
			// the index.

			target, flags, err := symlinks.Read(p)
			flags = flags & protocol.SymlinkTypeMask
			if err != nil {
				if debug {
					l.Debugln("readlink error:", p, err)
				}
				return skip
			}

			blocks, err := Blocks(strings.NewReader(target), w.BlockSize, 0)
			if err != nil {
				if debug {
					l.Debugln("hash link error:", p, err)
				}
				return skip
			}

			if w.CurrentFiler != nil {
				// A symlink is "unchanged", if
				//  - it exists
				//  - it wasn't deleted (because it isn't now)
				//  - it was a symlink
				//  - it wasn't invalid
				//  - the symlink type (file/dir) was the same
				//  - the block list (i.e. hash of target) was the same
				cf, ok = w.CurrentFiler.CurrentFile(rn)
				if ok && !cf.IsDeleted() && cf.IsSymlink() && !cf.IsInvalid() && SymlinkTypeEqual(flags, cf.Flags) && BlocksEqual(cf.Blocks, blocks) {
					return skip
				}
			}

			f := protocol.FileInfo{
				Name:     rn,
				Version:  cf.Version.Update(w.ShortID),
				Flags:    protocol.FlagSymlink | flags | protocol.FlagNoPermBits | 0666,
				Modified: 0,
				Blocks:   blocks,
			}

			if debug {
				l.Debugln("symlink to hash:", p, f)
			}

			fchan <- f

			return skip
		}

		if info.Mode().IsDir() {
			if w.CurrentFiler != nil {
				// A directory is "unchanged", if it
				//  - exists
				//  - has the same permissions as previously, unless we are ignoring permissions
				//  - was not marked deleted (since it apparently exists now)
				//  - was a directory previously (not a file or something else)
				//  - was not a symlink (since it's a directory now)
				//  - was not invalid (since it looks valid now)
				cf, ok = w.CurrentFiler.CurrentFile(rn)
				permUnchanged := w.IgnorePerms || !cf.HasPermissionBits() || PermsEqual(cf.Flags, uint32(info.Mode()))
				if ok && permUnchanged && !cf.IsDeleted() && cf.IsDirectory() && !cf.IsSymlink() && !cf.IsInvalid() {
					return nil
				}
			}

			flags := uint32(protocol.FlagDirectory)
			if w.IgnorePerms {
				flags |= protocol.FlagNoPermBits | 0777
			} else {
				flags |= uint32(info.Mode() & maskModePerm)
			}
			f := protocol.FileInfo{
				Name:     rn,
				Version:  cf.Version.Update(w.ShortID),
				Flags:    flags,
				Modified: mtime.Unix(),
			}
			if debug {
				l.Debugln("dir:", p, f)
			}
			fchan <- f
			return nil
		}

		if info.Mode().IsRegular() {
			curMode := uint32(info.Mode())
			if runtime.GOOS == "windows" && osutil.IsWindowsExecutable(rn) {
				curMode |= 0111
			}

			if w.CurrentFiler != nil {
				// A file is "unchanged", if it
				//  - exists
				//  - has the same permissions as previously, unless we are ignoring permissions
				//  - was not marked deleted (since it apparently exists now)
				//  - had the same modification time as it has now
				//  - was not a directory previously (since it's a file now)
				//  - was not a symlink (since it's a file now)
				//  - was not invalid (since it looks valid now)
				//  - has the same size as previously
				cf, ok = w.CurrentFiler.CurrentFile(rn)
				permUnchanged := w.IgnorePerms || !cf.HasPermissionBits() || PermsEqual(cf.Flags, curMode)
				if ok && permUnchanged && !cf.IsDeleted() && cf.Modified == mtime.Unix() && !cf.IsDirectory() &&
					!cf.IsSymlink() && !cf.IsInvalid() && cf.Size() == info.Size() {
					return nil
				}

				if debug {
					l.Debugln("rescan:", cf, mtime.Unix(), info.Mode()&os.ModePerm)
				}
			}

			var flags = curMode & uint32(maskModePerm)
			if w.IgnorePerms {
				flags = protocol.FlagNoPermBits | 0666
			}

			f := protocol.FileInfo{
				Name:     rn,
				Version:  cf.Version.Update(w.ShortID),
				Flags:    flags,
				Modified: mtime.Unix(),
			}
			if debug {
				l.Debugln("to hash:", p, f)
			}
			fchan <- f
		}

		return nil
	}
}

func checkDir(dir string) error {
	if info, err := osutil.Lstat(dir); err != nil {
		return err
	} else if !info.IsDir() {
		return errors.New(dir + ": not a directory")
	} else if debug {
		l.Debugln("checkDir", dir, info)
	}
	return nil
}

func PermsEqual(a, b uint32) bool {
	switch runtime.GOOS {
	case "windows":
		// There is only writeable and read only, represented for user, group
		// and other equally. We only compare against user.
		return a&0600 == b&0600
	default:
		// All bits count
		return a&0777 == b&0777
	}
}

func SymlinkTypeEqual(disk, index uint32) bool {
	// If the target is missing, Unix never knows what type of symlink it is
	// and Windows always knows even if there is no target. Which means that
	// without this special check a Unix node would be fighting with a Windows
	// node about whether or not the target is known. Basically, if you don't
	// know and someone else knows, just accept it. The fact that you don't
	// know means you are on Unix, and on Unix you don't really care what the
	// target type is. The moment you do know, and if something doesn't match,
	// that will propagate through the cluster.
	if disk&protocol.FlagSymlinkMissingTarget != 0 && index&protocol.FlagSymlinkMissingTarget == 0 {
		return true
	}
	return disk&protocol.SymlinkTypeMask == index&protocol.SymlinkTypeMask
}
