// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reservefs.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb/util"
)

var errFileOpen = errors.New("leveldb/storage: file still open")

type fileLock interface {
	release() error
}

type fileStorageLock struct {
	fs *fileStorage
}

func (lock *fileStorageLock) Release() {
	fs := lock.fs
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.slock == lock {
		fs.slock = nil
	}
	return
}

// fileStorage is a file-system backed storage.
type fileStorage struct {
	path string

	mu    sync.Mutex
	flock fileLock
	slock *fileStorageLock
	logw  *os.File
	buf   []byte
	// Opened file counter; if open < 0 means closed.
	open int
	day  int
}

// OpenFile returns a new filesytem-backed storage implementation with the given
// path. This also hold a file lock, so any subsequent attempt to open the same
// path will fail.
//
// The storage must be closed after use, by calling Close method.
func OpenFile(path string) (Storage, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}

	flock, err := newFileLock(filepath.Join(path, "LOCK"))
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			flock.release()
		}
	}()

	rename(filepath.Join(path, "LOG"), filepath.Join(path, "LOG.old"))
	logw, err := os.OpenFile(filepath.Join(path, "LOG"), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	fs := &fileStorage{path: path, flock: flock, logw: logw}
	runtime.SetFinalizer(fs, (*fileStorage).Close)
	return fs, nil
}

func (fs *fileStorage) Lock() (util.Releaser, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	if fs.slock != nil {
		return nil, ErrLocked
	}
	fs.slock = &fileStorageLock{fs: fs}
	return fs.slock, nil
}

func itoa(buf []byte, i int, wid int) []byte {
	var u uint = uint(i)
	if u == 0 && wid <= 1 {
		return append(buf, '0')
	}

	// Assemble decimal in reverse order.
	var b [32]byte
	bp := len(b)
	for ; u > 0 || wid > 0; u /= 10 {
		bp--
		wid--
		b[bp] = byte(u%10) + '0'
	}
	return append(buf, b[bp:]...)
}

func (fs *fileStorage) printDay(t time.Time) {
	if fs.day == t.Day() {
		return
	}
	fs.day = t.Day()
	fs.logw.Write([]byte("=============== " + t.Format("Jan 2, 2006 (MST)") + " ===============\n"))
}

func (fs *fileStorage) doLog(t time.Time, str string) {
	fs.printDay(t)
	hour, min, sec := t.Clock()
	msec := t.Nanosecond() / 1e3
	// time
	fs.buf = itoa(fs.buf[:0], hour, 2)
	fs.buf = append(fs.buf, ':')
	fs.buf = itoa(fs.buf, min, 2)
	fs.buf = append(fs.buf, ':')
	fs.buf = itoa(fs.buf, sec, 2)
	fs.buf = append(fs.buf, '.')
	fs.buf = itoa(fs.buf, msec, 6)
	fs.buf = append(fs.buf, ' ')
	// write
	fs.buf = append(fs.buf, []byte(str)...)
	fs.buf = append(fs.buf, '\n')
	fs.logw.Write(fs.buf)
}

func (fs *fileStorage) Log(str string) {
	t := time.Now()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return
	}
	fs.doLog(t, str)
}

func (fs *fileStorage) log(str string) {
	fs.doLog(time.Now(), str)
}

func (fs *fileStorage) GetFile(num uint64, t FileType) File {
	return &file{fs: fs, num: num, t: t}
}

func (fs *fileStorage) GetFiles(t FileType) (ff []File, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	dir, err := os.Open(fs.path)
	if err != nil {
		return
	}
	fnn, err := dir.Readdirnames(0)
	// Close the dir first before checking for Readdirnames error.
	if err := dir.Close(); err != nil {
		fs.log(fmt.Sprintf("close dir: %v", err))
	}
	if err != nil {
		return
	}
	f := &file{fs: fs}
	for _, fn := range fnn {
		if f.parse(fn) && (f.t&t) != 0 {
			ff = append(ff, f)
			f = &file{fs: fs}
		}
	}
	return
}

func (fs *fileStorage) GetManifest() (f File, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	dir, err := os.Open(fs.path)
	if err != nil {
		return
	}
	fnn, err := dir.Readdirnames(0)
	// Close the dir first before checking for Readdirnames error.
	if err := dir.Close(); err != nil {
		fs.log(fmt.Sprintf("close dir: %v", err))
	}
	if err != nil {
		return
	}
	// Find latest CURRENT file.
	var rem []string
	var pend bool
	var cerr error
	for _, fn := range fnn {
		if strings.HasPrefix(fn, "CURRENT") {
			pend1 := len(fn) > 7
			// Make sure it is valid name for a CURRENT file, otherwise skip it.
			if pend1 {
				if fn[7] != '.' || len(fn) < 9 {
					fs.log(fmt.Sprintf("skipping %s: invalid file name", fn))
					continue
				}
				if _, e1 := strconv.ParseUint(fn[8:], 10, 0); e1 != nil {
					fs.log(fmt.Sprintf("skipping %s: invalid file num: %v", fn, e1))
					continue
				}
			}
			path := filepath.Join(fs.path, fn)
			r, e1 := os.OpenFile(path, os.O_RDONLY, 0)
			if e1 != nil {
				return nil, e1
			}
			b, e1 := ioutil.ReadAll(r)
			if e1 != nil {
				r.Close()
				return nil, e1
			}
			f1 := &file{fs: fs}
			if len(b) < 1 || b[len(b)-1] != '\n' || !f1.parse(string(b[:len(b)-1])) {
				fs.log(fmt.Sprintf("skipping %s: corrupted or incomplete", fn))
				if pend1 {
					rem = append(rem, fn)
				}
				if !pend1 || cerr == nil {
					cerr = &ErrCorrupted{
						File: fsParseName(filepath.Base(fn)),
						Err:  errors.New("leveldb/storage: corrupted or incomplete manifest file"),
					}
				}
			} else if f != nil && f1.Num() < f.Num() {
				fs.log(fmt.Sprintf("skipping %s: obsolete", fn))
				if pend1 {
					rem = append(rem, fn)
				}
			} else {
				f = f1
				pend = pend1
			}
			if err := r.Close(); err != nil {
				fs.log(fmt.Sprintf("close %s: %v", fn, err))
			}
		}
	}
	// Don't remove any files if there is no valid CURRENT file.
	if f == nil {
		if cerr != nil {
			err = cerr
		} else {
			err = os.ErrNotExist
		}
		return
	}
	// Rename pending CURRENT file to an effective CURRENT.
	if pend {
		path := fmt.Sprintf("%s.%d", filepath.Join(fs.path, "CURRENT"), f.Num())
		if err := rename(path, filepath.Join(fs.path, "CURRENT")); err != nil {
			fs.log(fmt.Sprintf("CURRENT.%d -> CURRENT: %v", f.Num(), err))
		}
	}
	// Remove obsolete or incomplete pending CURRENT files.
	for _, fn := range rem {
		path := filepath.Join(fs.path, fn)
		if err := os.Remove(path); err != nil {
			fs.log(fmt.Sprintf("remove %s: %v", fn, err))
		}
	}
	return
}

func (fs *fileStorage) SetManifest(f File) (err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return ErrClosed
	}
	f2, ok := f.(*file)
	if !ok || f2.t != TypeManifest {
		return ErrInvalidFile
	}
	defer func() {
		if err != nil {
			fs.log(fmt.Sprintf("CURRENT: %v", err))
		}
	}()
	path := fmt.Sprintf("%s.%d", filepath.Join(fs.path, "CURRENT"), f2.Num())
	w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, f2.name())
	// Close the file first.
	if err := w.Close(); err != nil {
		fs.log(fmt.Sprintf("close CURRENT.%d: %v", f2.num, err))
	}
	if err != nil {
		return err
	}
	return rename(path, filepath.Join(fs.path, "CURRENT"))
}

func (fs *fileStorage) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return ErrClosed
	}
	// Clear the finalizer.
	runtime.SetFinalizer(fs, nil)

	if fs.open > 0 {
		fs.log(fmt.Sprintf("close: warning, %d files still open", fs.open))
	}
	fs.open = -1
	e1 := fs.logw.Close()
	err := fs.flock.release()
	if err == nil {
		err = e1
	}
	return err
}

type fileWrap struct {
	*os.File
	f *file
}

func (fw fileWrap) Sync() error {
	if err := fw.File.Sync(); err != nil {
		return err
	}
	if fw.f.Type() == TypeManifest {
		// Also sync parent directory if file type is manifest.
		// See: https://code.google.com/p/leveldb/issues/detail?id=190.
		if err := syncDir(fw.f.fs.path); err != nil {
			return err
		}
	}
	return nil
}

func (fw fileWrap) Close() error {
	f := fw.f
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if !f.open {
		return ErrClosed
	}
	f.open = false
	f.fs.open--
	err := fw.File.Close()
	if err != nil {
		f.fs.log(fmt.Sprintf("close %s.%d: %v", f.Type(), f.Num(), err))
	}
	return err
}

type file struct {
	fs   *fileStorage
	num  uint64
	t    FileType
	open bool
}

func (f *file) Open() (Reader, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.fs.open < 0 {
		return nil, ErrClosed
	}
	if f.open {
		return nil, errFileOpen
	}
	of, err := os.OpenFile(f.path(), os.O_RDONLY, 0)
	if err != nil {
		if f.hasOldName() && os.IsNotExist(err) {
			of, err = os.OpenFile(f.oldPath(), os.O_RDONLY, 0)
			if err == nil {
				goto ok
			}
		}
		return nil, err
	}
ok:
	f.open = true
	f.fs.open++
	return fileWrap{of, f}, nil
}

func (f *file) Create() (Writer, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.fs.open < 0 {
		return nil, ErrClosed
	}
	if f.open {
		return nil, errFileOpen
	}
	of, err := os.OpenFile(f.path(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	f.open = true
	f.fs.open++
	return fileWrap{of, f}, nil
}

func (f *file) Replace(newfile File) error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.fs.open < 0 {
		return ErrClosed
	}
	newfile2, ok := newfile.(*file)
	if !ok {
		return ErrInvalidFile
	}
	if f.open || newfile2.open {
		return errFileOpen
	}
	return rename(newfile2.path(), f.path())
}

func (f *file) Type() FileType {
	return f.t
}

func (f *file) Num() uint64 {
	return f.num
}

func (f *file) Remove() error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.fs.open < 0 {
		return ErrClosed
	}
	if f.open {
		return errFileOpen
	}
	err := os.Remove(f.path())
	if err != nil {
		f.fs.log(fmt.Sprintf("remove %s.%d: %v", f.Type(), f.Num(), err))
	}
	// Also try remove file with old name, just in case.
	if f.hasOldName() {
		if e1 := os.Remove(f.oldPath()); !os.IsNotExist(e1) {
			f.fs.log(fmt.Sprintf("remove %s.%d: %v (old name)", f.Type(), f.Num(), err))
			err = e1
		}
	}
	return err
}

func (f *file) hasOldName() bool {
	return f.t == TypeTable
}

func (f *file) oldName() string {
	switch f.t {
	case TypeTable:
		return fmt.Sprintf("%06d.sst", f.num)
	}
	return f.name()
}

func (f *file) oldPath() string {
	return filepath.Join(f.fs.path, f.oldName())
}

func (f *file) name() string {
	switch f.t {
	case TypeManifest:
		return fmt.Sprintf("MANIFEST-%06d", f.num)
	case TypeJournal:
		return fmt.Sprintf("%06d.log", f.num)
	case TypeTable:
		return fmt.Sprintf("%06d.ldb", f.num)
	case TypeTemp:
		return fmt.Sprintf("%06d.tmp", f.num)
	default:
		panic("invalid file type")
	}
}

func (f *file) path() string {
	return filepath.Join(f.fs.path, f.name())
}

func fsParseName(name string) *FileInfo {
	fi := &FileInfo{}
	var tail string
	_, err := fmt.Sscanf(name, "%d.%s", &fi.Num, &tail)
	if err == nil {
		switch tail {
		case "log":
			fi.Type = TypeJournal
		case "ldb", "sst":
			fi.Type = TypeTable
		case "tmp":
			fi.Type = TypeTemp
		default:
			return nil
		}
		return fi
	}
	n, _ := fmt.Sscanf(name, "MANIFEST-%d%s", &fi.Num, &tail)
	if n == 1 {
		fi.Type = TypeManifest
		return fi
	}
	return nil
}

func (f *file) parse(name string) bool {
	fi := fsParseName(name)
	if fi == nil {
		return false
	}
	f.t = fi.Type
	f.num = fi.Num
	return true
}
