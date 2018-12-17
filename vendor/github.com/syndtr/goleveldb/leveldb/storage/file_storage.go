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
)

var (
	errFileOpen = errors.New("leveldb/storage: file still open")
	errReadOnly = errors.New("leveldb/storage: storage is read-only")
)

type fileLock interface {
	release() error
}

type fileStorageLock struct {
	fs *fileStorage
}

func (lock *fileStorageLock) Unlock() {
	if lock.fs != nil {
		lock.fs.mu.Lock()
		defer lock.fs.mu.Unlock()
		if lock.fs.slock == lock {
			lock.fs.slock = nil
		}
	}
}

const logSizeThreshold = 1024 * 1024 // 1 MiB

// fileStorage is a file-system backed storage.
type fileStorage struct {
	path     string
	readOnly bool

	mu      sync.Mutex
	flock   fileLock
	slock   *fileStorageLock
	logw    *os.File
	logSize int64
	buf     []byte
	// Opened file counter; if open < 0 means closed.
	open int
	day  int
}

// OpenFile returns a new filesytem-backed storage implementation with the given
// path. This also acquire a file lock, so any subsequent attempt to open the
// same path will fail.
//
// The storage must be closed after use, by calling Close method.
func OpenFile(path string, readOnly bool) (Storage, error) {
	if fi, err := os.Stat(path); err == nil {
		if !fi.IsDir() {
			return nil, fmt.Errorf("leveldb/storage: open %s: not a directory", path)
		}
	} else if os.IsNotExist(err) && !readOnly {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	flock, err := newFileLock(filepath.Join(path, "LOCK"), readOnly)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			flock.release()
		}
	}()

	var (
		logw    *os.File
		logSize int64
	)
	if !readOnly {
		logw, err = os.OpenFile(filepath.Join(path, "LOG"), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}
		logSize, err = logw.Seek(0, os.SEEK_END)
		if err != nil {
			logw.Close()
			return nil, err
		}
	}

	fs := &fileStorage{
		path:     path,
		readOnly: readOnly,
		flock:    flock,
		logw:     logw,
		logSize:  logSize,
	}
	runtime.SetFinalizer(fs, (*fileStorage).Close)
	return fs, nil
}

func (fs *fileStorage) Lock() (Locker, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	if fs.readOnly {
		return &fileStorageLock{}, nil
	}
	if fs.slock != nil {
		return nil, ErrLocked
	}
	fs.slock = &fileStorageLock{fs: fs}
	return fs.slock, nil
}

func itoa(buf []byte, i int, wid int) []byte {
	u := uint(i)
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
	if fs.logSize > logSizeThreshold {
		// Rotate log file.
		fs.logw.Close()
		fs.logw = nil
		fs.logSize = 0
		rename(filepath.Join(fs.path, "LOG"), filepath.Join(fs.path, "LOG.old"))
	}
	if fs.logw == nil {
		var err error
		fs.logw, err = os.OpenFile(filepath.Join(fs.path, "LOG"), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return
		}
		// Force printDay on new log file.
		fs.day = 0
	}
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
	if !fs.readOnly {
		t := time.Now()
		fs.mu.Lock()
		defer fs.mu.Unlock()
		if fs.open < 0 {
			return
		}
		fs.doLog(t, str)
	}
}

func (fs *fileStorage) log(str string) {
	if !fs.readOnly {
		fs.doLog(time.Now(), str)
	}
}

func (fs *fileStorage) SetMeta(fd FileDesc) (err error) {
	if !FileDescOk(fd) {
		return ErrInvalidFile
	}
	if fs.readOnly {
		return errReadOnly
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return ErrClosed
	}
	defer func() {
		if err != nil {
			fs.log(fmt.Sprintf("CURRENT: %v", err))
		}
	}()
	path := fmt.Sprintf("%s.%d", filepath.Join(fs.path, "CURRENT"), fd.Num)
	w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	_, err = fmt.Fprintln(w, fsGenName(fd))
	if err != nil {
		fs.log(fmt.Sprintf("write CURRENT.%d: %v", fd.Num, err))
		return
	}
	if err = w.Sync(); err != nil {
		fs.log(fmt.Sprintf("flush CURRENT.%d: %v", fd.Num, err))
		return
	}
	if err = w.Close(); err != nil {
		fs.log(fmt.Sprintf("close CURRENT.%d: %v", fd.Num, err))
		return
	}
	if err != nil {
		return
	}
	if err = rename(path, filepath.Join(fs.path, "CURRENT")); err != nil {
		fs.log(fmt.Sprintf("rename CURRENT.%d: %v", fd.Num, err))
		return
	}
	// Sync root directory.
	if err = syncDir(fs.path); err != nil {
		fs.log(fmt.Sprintf("syncDir: %v", err))
	}
	return
}

func (fs *fileStorage) GetMeta() (fd FileDesc, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return FileDesc{}, ErrClosed
	}
	dir, err := os.Open(fs.path)
	if err != nil {
		return
	}
	names, err := dir.Readdirnames(0)
	// Close the dir first before checking for Readdirnames error.
	if ce := dir.Close(); ce != nil {
		fs.log(fmt.Sprintf("close dir: %v", ce))
	}
	if err != nil {
		return
	}
	// Find latest CURRENT file.
	var rem []string
	var pend bool
	var cerr error
	for _, name := range names {
		if strings.HasPrefix(name, "CURRENT") {
			pend1 := len(name) > 7
			var pendNum int64
			// Make sure it is valid name for a CURRENT file, otherwise skip it.
			if pend1 {
				if name[7] != '.' || len(name) < 9 {
					fs.log(fmt.Sprintf("skipping %s: invalid file name", name))
					continue
				}
				var e1 error
				if pendNum, e1 = strconv.ParseInt(name[8:], 10, 0); e1 != nil {
					fs.log(fmt.Sprintf("skipping %s: invalid file num: %v", name, e1))
					continue
				}
			}
			path := filepath.Join(fs.path, name)
			r, e1 := os.OpenFile(path, os.O_RDONLY, 0)
			if e1 != nil {
				return FileDesc{}, e1
			}
			b, e1 := ioutil.ReadAll(r)
			if e1 != nil {
				r.Close()
				return FileDesc{}, e1
			}
			var fd1 FileDesc
			if len(b) < 1 || b[len(b)-1] != '\n' || !fsParseNamePtr(string(b[:len(b)-1]), &fd1) {
				fs.log(fmt.Sprintf("skipping %s: corrupted or incomplete", name))
				if pend1 {
					rem = append(rem, name)
				}
				if !pend1 || cerr == nil {
					metaFd, _ := fsParseName(name)
					cerr = &ErrCorrupted{
						Fd:  metaFd,
						Err: errors.New("leveldb/storage: corrupted or incomplete meta file"),
					}
				}
			} else if pend1 && pendNum != fd1.Num {
				fs.log(fmt.Sprintf("skipping %s: inconsistent pending-file num: %d vs %d", name, pendNum, fd1.Num))
				rem = append(rem, name)
			} else if fd1.Num < fd.Num {
				fs.log(fmt.Sprintf("skipping %s: obsolete", name))
				if pend1 {
					rem = append(rem, name)
				}
			} else {
				fd = fd1
				pend = pend1
			}
			if err := r.Close(); err != nil {
				fs.log(fmt.Sprintf("close %s: %v", name, err))
			}
		}
	}
	// Don't remove any files if there is no valid CURRENT file.
	if fd.Zero() {
		if cerr != nil {
			err = cerr
		} else {
			err = os.ErrNotExist
		}
		return
	}
	if !fs.readOnly {
		// Rename pending CURRENT file to an effective CURRENT.
		if pend {
			path := fmt.Sprintf("%s.%d", filepath.Join(fs.path, "CURRENT"), fd.Num)
			if err := rename(path, filepath.Join(fs.path, "CURRENT")); err != nil {
				fs.log(fmt.Sprintf("CURRENT.%d -> CURRENT: %v", fd.Num, err))
			}
		}
		// Remove obsolete or incomplete pending CURRENT files.
		for _, name := range rem {
			path := filepath.Join(fs.path, name)
			if err := os.Remove(path); err != nil {
				fs.log(fmt.Sprintf("remove %s: %v", name, err))
			}
		}
	}
	return
}

func (fs *fileStorage) List(ft FileType) (fds []FileDesc, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	dir, err := os.Open(fs.path)
	if err != nil {
		return
	}
	names, err := dir.Readdirnames(0)
	// Close the dir first before checking for Readdirnames error.
	if cerr := dir.Close(); cerr != nil {
		fs.log(fmt.Sprintf("close dir: %v", cerr))
	}
	if err == nil {
		for _, name := range names {
			if fd, ok := fsParseName(name); ok && fd.Type&ft != 0 {
				fds = append(fds, fd)
			}
		}
	}
	return
}

func (fs *fileStorage) Open(fd FileDesc) (Reader, error) {
	if !FileDescOk(fd) {
		return nil, ErrInvalidFile
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	of, err := os.OpenFile(filepath.Join(fs.path, fsGenName(fd)), os.O_RDONLY, 0)
	if err != nil {
		if fsHasOldName(fd) && os.IsNotExist(err) {
			of, err = os.OpenFile(filepath.Join(fs.path, fsGenOldName(fd)), os.O_RDONLY, 0)
			if err == nil {
				goto ok
			}
		}
		return nil, err
	}
ok:
	fs.open++
	return &fileWrap{File: of, fs: fs, fd: fd}, nil
}

func (fs *fileStorage) Create(fd FileDesc) (Writer, error) {
	if !FileDescOk(fd) {
		return nil, ErrInvalidFile
	}
	if fs.readOnly {
		return nil, errReadOnly
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return nil, ErrClosed
	}
	of, err := os.OpenFile(filepath.Join(fs.path, fsGenName(fd)), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	fs.open++
	return &fileWrap{File: of, fs: fs, fd: fd}, nil
}

func (fs *fileStorage) Remove(fd FileDesc) error {
	if !FileDescOk(fd) {
		return ErrInvalidFile
	}
	if fs.readOnly {
		return errReadOnly
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return ErrClosed
	}
	err := os.Remove(filepath.Join(fs.path, fsGenName(fd)))
	if err != nil {
		if fsHasOldName(fd) && os.IsNotExist(err) {
			if e1 := os.Remove(filepath.Join(fs.path, fsGenOldName(fd))); !os.IsNotExist(e1) {
				fs.log(fmt.Sprintf("remove %s: %v (old name)", fd, err))
				err = e1
			}
		} else {
			fs.log(fmt.Sprintf("remove %s: %v", fd, err))
		}
	}
	return err
}

func (fs *fileStorage) Rename(oldfd, newfd FileDesc) error {
	if !FileDescOk(oldfd) || !FileDescOk(newfd) {
		return ErrInvalidFile
	}
	if oldfd == newfd {
		return nil
	}
	if fs.readOnly {
		return errReadOnly
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.open < 0 {
		return ErrClosed
	}
	return rename(filepath.Join(fs.path, fsGenName(oldfd)), filepath.Join(fs.path, fsGenName(newfd)))
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
	if fs.logw != nil {
		fs.logw.Close()
	}
	return fs.flock.release()
}

type fileWrap struct {
	*os.File
	fs     *fileStorage
	fd     FileDesc
	closed bool
}

func (fw *fileWrap) Sync() error {
	if err := fw.File.Sync(); err != nil {
		return err
	}
	if fw.fd.Type == TypeManifest {
		// Also sync parent directory if file type is manifest.
		// See: https://code.google.com/p/leveldb/issues/detail?id=190.
		if err := syncDir(fw.fs.path); err != nil {
			fw.fs.log(fmt.Sprintf("syncDir: %v", err))
			return err
		}
	}
	return nil
}

func (fw *fileWrap) Close() error {
	fw.fs.mu.Lock()
	defer fw.fs.mu.Unlock()
	if fw.closed {
		return ErrClosed
	}
	fw.closed = true
	fw.fs.open--
	err := fw.File.Close()
	if err != nil {
		fw.fs.log(fmt.Sprintf("close %s: %v", fw.fd, err))
	}
	return err
}

func fsGenName(fd FileDesc) string {
	switch fd.Type {
	case TypeManifest:
		return fmt.Sprintf("MANIFEST-%06d", fd.Num)
	case TypeJournal:
		return fmt.Sprintf("%06d.log", fd.Num)
	case TypeTable:
		return fmt.Sprintf("%06d.ldb", fd.Num)
	case TypeTemp:
		return fmt.Sprintf("%06d.tmp", fd.Num)
	default:
		panic("invalid file type")
	}
}

func fsHasOldName(fd FileDesc) bool {
	return fd.Type == TypeTable
}

func fsGenOldName(fd FileDesc) string {
	switch fd.Type {
	case TypeTable:
		return fmt.Sprintf("%06d.sst", fd.Num)
	}
	return fsGenName(fd)
}

func fsParseName(name string) (fd FileDesc, ok bool) {
	var tail string
	_, err := fmt.Sscanf(name, "%d.%s", &fd.Num, &tail)
	if err == nil {
		switch tail {
		case "log":
			fd.Type = TypeJournal
		case "ldb", "sst":
			fd.Type = TypeTable
		case "tmp":
			fd.Type = TypeTemp
		default:
			return
		}
		return fd, true
	}
	n, _ := fmt.Sscanf(name, "MANIFEST-%d%s", &fd.Num, &tail)
	if n == 1 {
		fd.Type = TypeManifest
		return fd, true
	}
	return
}

func fsParseNamePtr(name string, fd *FileDesc) bool {
	_fd, ok := fsParseName(name)
	if fd != nil {
		*fd = _fd
	}
	return ok
}
