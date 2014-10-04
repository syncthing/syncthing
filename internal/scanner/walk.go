// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

	"code.google.com/p/go.text/unicode/norm"

	"github.com/syncthing/syncthing/internal/ignore"
	"github.com/syncthing/syncthing/internal/lamport"
	"github.com/syncthing/syncthing/internal/protocol"
)

type Walker struct {
	// Dir is the base directory for the walk
	Dir string
	// Limit walking to this path within Dir, or no limit if Sub is blank
	Sub string
	// BlockSize controls the size of the block used when hashing.
	BlockSize int
	// List of patterns to ignore
	Ignores ignore.Patterns
	// If TempNamer is not nil, it is used to ignore tempory files when walking.
	TempNamer TempNamer
	// If CurrentFiler is not nil, it is queried for the current file before rescanning.
	CurrentFiler CurrentFiler
	// If IgnorePerms is true, changes to permission bits will not be
	// detected. Scanned files will get zero permission bits and the
	// NoPermissionBits flag set.
	IgnorePerms bool
}

type TempNamer interface {
	// Temporary returns a temporary name for the filed referred to by filepath.
	TempName(path string) string
	// IsTemporary returns true if path refers to the name of temporary file.
	IsTemporary(path string) bool
}

type CurrentFiler interface {
	// CurrentFile returns the file as seen at last scan.
	CurrentFile(name string) protocol.FileInfo
}

// Walk returns the list of files found in the local folder by scanning the
// file system. Files are blockwise hashed.
func (w *Walker) Walk() (chan protocol.FileInfo, error) {
	if debug {
		l.Debugln("Walk", w.Dir, w.Sub, w.BlockSize, w.Ignores)
	}

	err := checkDir(w.Dir)
	if err != nil {
		return nil, err
	}

	files := make(chan protocol.FileInfo)
	hashedFiles := make(chan protocol.FileInfo)
	newParallelHasher(w.Dir, w.BlockSize, runtime.NumCPU(), hashedFiles, files)

	go func() {
		hashFiles := w.walkAndHashFiles(files)
		filepath.Walk(filepath.Join(w.Dir, w.Sub), hashFiles)
		close(files)
	}()

	return hashedFiles, nil
}

func (w *Walker) walkAndHashFiles(fchan chan protocol.FileInfo) filepath.WalkFunc {
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
			return nil
		}

		if sn := filepath.Base(rn); sn == ".stignore" || sn == ".stversions" || w.Ignores.Match(rn) {
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

		if info.Mode().IsDir() {
			if w.CurrentFiler != nil {
				cf := w.CurrentFiler.CurrentFile(rn)
				permUnchanged := w.IgnorePerms || !protocol.HasPermissionBits(cf.Flags) || PermsEqual(cf.Flags, uint32(info.Mode()))
				if !protocol.IsDeleted(cf.Flags) && protocol.IsDirectory(cf.Flags) && permUnchanged {
					return nil
				}
			}

			var flags uint32 = protocol.FlagDirectory
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
				l.Debugln("dir:", f)
			}
			fchan <- f
			return nil
		}

		if info.Mode().IsRegular() {
			if w.CurrentFiler != nil {
				cf := w.CurrentFiler.CurrentFile(rn)
				permUnchanged := w.IgnorePerms || !protocol.HasPermissionBits(cf.Flags) || PermsEqual(cf.Flags, uint32(info.Mode()))
				if !protocol.IsDeleted(cf.Flags) && cf.Modified == info.ModTime().Unix() && permUnchanged {
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

			fchan <- protocol.FileInfo{
				Name:     rn,
				Version:  lamport.Default.Tick(0),
				Flags:    flags,
				Modified: info.ModTime().Unix(),
			}
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
