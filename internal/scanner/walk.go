// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/ignore"
	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/symlinks"
	"golang.org/x/text/unicode/norm"
)

type Walker struct {
	// Dir is the base directory for the walk
	Dir string
	// Limit walking to this path within Dir, or no limit if Sub is blank
	Sub string
	// BlockSize controls the size of the block used when hashing.
	BlockSize int
	// If Matcher is not nil, it is used to identify files to ignore which were specified by the user.
	Matcher *ignore.Matcher
	// If TempNamer is not nil, it is used to ignore tempory files when walking.
	TempNamer TempNamer
	// Number of hours to keep temporary files for
	TempLifetime time.Duration
	// If CurrentFiler is not nil, it is queried for the current file before rescanning.
	CurrentFiler CurrentFiler
	// If IgnorePerms is true, changes to permission bits will not be
	// detected. Scanned files will get zero permission bits and the
	// NoPermissionBits flag set.
	IgnorePerms bool
	// Number of routines to use for hashing
	Hashers int
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
		l.Debugln("Walk", w.Dir, w.Sub, w.BlockSize, w.Matcher)
	}

	err := checkDir(w.Dir)
	if err != nil {
		return nil, err
	}

	workers := w.Hashers
	if workers < 1 {
		workers = runtime.NumCPU()
	}

	files := make(chan protocol.FileInfo)
	hashedFiles := make(chan protocol.FileInfo)
	newParallelHasher(w.Dir, w.BlockSize, workers, hashedFiles, files)

	go func() {
		hashFiles := w.walkAndHashFiles(files)
		filepath.Walk(filepath.Join(w.Dir, w.Sub), hashFiles)
		close(files)
	}()

	return hashedFiles, nil
}

func (w *Walker) walkAndHashFiles(fchan chan protocol.FileInfo) filepath.WalkFunc {
	now := time.Now()
	return func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if debug {
				l.Debugln("error:", p, info, err)
			}
			return nil
		}

		rn, err := filepath.Rel(w.Dir, p)
		if err != nil {
			if debug {
				l.Debugln("rel error:", p, err)
			}
			return nil
		}

		if rn == "." {
			return nil
		}

		if w.TempNamer != nil && w.TempNamer.IsTemporary(rn) {
			// A temporary file
			if debug {
				l.Debugln("temporary:", rn)
			}
			if info.Mode().IsRegular() && info.ModTime().Add(w.TempLifetime).Before(now) {
				os.Remove(p)
				if debug {
					l.Debugln("removing temporary:", rn, info.ModTime())
				}
			}
			return nil
		}

		if sn := filepath.Base(rn); sn == ".stignore" || sn == ".stfolder" ||
			strings.HasPrefix(rn, ".stversions") || (w.Matcher != nil && w.Matcher.Match(rn)) {
			// An ignored file
			if debug {
				l.Debugln("ignored:", rn)
			}
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if (runtime.GOOS == "linux" || runtime.GOOS == "windows") && !norm.NFC.IsNormalString(rn) {
			l.Warnf("File %q contains non-NFC UTF-8 sequences and cannot be synced. Consider renaming.", rn)
			return nil
		}

		// Index wise symlinks are always files, regardless of what the target
		// is, because symlinks carry their target path as their content.
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			var rval error
			// If the target is a directory, do NOT descend down there. This
			// will cause files to get tracked, and removing the symlink will
			// as a result remove files in their real location. But do not
			// SkipDir if the target is not a directory, as it will stop
			// scanning the current directory.
			if info.IsDir() {
				rval = filepath.SkipDir
			}

			// If we don't support symlinks, skip.
			if !symlinks.Supported {
				return rval
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
				return rval
			}

			blocks, err := Blocks(strings.NewReader(target), w.BlockSize, 0)
			if err != nil {
				if debug {
					l.Debugln("hash link error:", p, err)
				}
				return rval
			}

			if w.CurrentFiler != nil {
				// A symlink is "unchanged", if
				//  - it exists
				//  - it wasn't deleted (because it isn't now)
				//  - it was a symlink
				//  - it wasn't invalid
				//  - the symlink type (file/dir) was the same
				//  - the block list (i.e. hash of target) was the same
				cf, ok := w.CurrentFiler.CurrentFile(rn)
				if ok && !cf.IsDeleted() && cf.IsSymlink() && !cf.IsInvalid() && SymlinkTypeEqual(flags, cf.Flags) && BlocksEqual(cf.Blocks, blocks) {
					return rval
				}
			}

			f := protocol.FileInfo{
				Name:     rn,
				Version:  lamport.Default.Tick(0),
				Flags:    protocol.FlagSymlink | flags | protocol.FlagNoPermBits | 0666,
				Modified: 0,
				Blocks:   blocks,
			}

			if debug {
				l.Debugln("symlink to hash:", p, f)
			}

			fchan <- f

			return rval
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
				cf, ok := w.CurrentFiler.CurrentFile(rn)
				permUnchanged := w.IgnorePerms || !cf.HasPermissionBits() || PermsEqual(cf.Flags, uint32(info.Mode()))
				if ok && permUnchanged && !cf.IsDeleted() && cf.IsDirectory() && !cf.IsSymlink() && !cf.IsInvalid() {
					return nil
				}
			}

			flags := uint32(protocol.FlagDirectory)
			if w.IgnorePerms {
				flags |= protocol.FlagNoPermBits | 0777
			} else {
				flags |= uint32(info.Mode() & os.ModePerm)
			}
			f := protocol.FileInfo{
				Name:     rn,
				Version:  lamport.Default.Tick(0),
				Flags:    flags,
				Modified: info.ModTime().Unix(),
			}
			if debug {
				l.Debugln("dir:", p, f)
			}
			fchan <- f
			return nil
		}

		if info.Mode().IsRegular() {
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
				cf, ok := w.CurrentFiler.CurrentFile(rn)
				permUnchanged := w.IgnorePerms || !cf.HasPermissionBits() || PermsEqual(cf.Flags, uint32(info.Mode()))
				if ok && permUnchanged && !cf.IsDeleted() && cf.Modified == info.ModTime().Unix() && !cf.IsDirectory() &&
					!cf.IsSymlink() && !cf.IsInvalid() && cf.Size() == info.Size() {
					return nil
				}

				if debug {
					l.Debugln("rescan:", cf, info.ModTime().Unix(), info.Mode()&os.ModePerm)
				}
			}

			var flags = uint32(info.Mode() & os.ModePerm)
			if w.IgnorePerms {
				flags = protocol.FlagNoPermBits | 0666
			}

			f := protocol.FileInfo{
				Name:     rn,
				Version:  lamport.Default.Tick(0),
				Flags:    flags,
				Modified: info.ModTime().Unix(),
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
	if info, err := os.Lstat(dir); err != nil {
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

// If the target is missing, Unix never knows what type of symlink it is
// and Windows always knows even if there is no target.
// Which means that without this special check a Unix node would be fighting
// with a Windows node about whether or not the target is known.
// Basically, if you don't know and someone else knows, just accept it.
// The fact that you don't know means you are on Unix, and on Unix you don't
// really care what the target type is. The moment you do know, and if something
// doesn't match, that will propogate throught the cluster.
func SymlinkTypeEqual(disk, index uint32) bool {
	if disk&protocol.FlagSymlinkMissingTarget != 0 && index&protocol.FlagSymlinkMissingTarget == 0 {
		return true
	}
	return disk&protocol.SymlinkTypeMask == index&protocol.SymlinkTypeMask

}
