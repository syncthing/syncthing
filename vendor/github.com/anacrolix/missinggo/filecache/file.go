package filecache

import (
	"errors"
	"math"
	"os"
	"sync"
	"time"

	"github.com/anacrolix/missinggo/pproffd"
)

type File struct {
	mu   sync.Mutex
	c    *Cache
	path string
	f    pproffd.OSFile
	gone bool
}

func (me *File) Remove() (err error) {
	return me.c.Remove(me.path)
}

func (me *File) Seek(offset int64, whence int) (ret int64, err error) {
	ret, err = me.f.Seek(offset, whence)
	return
}

func (me *File) maxWrite() (max int64, err error) {
	if me.c.capacity < 0 {
		max = math.MaxInt64
		return
	}
	pos, err := me.Seek(0, os.SEEK_CUR)
	if err != nil {
		return
	}
	max = me.c.capacity - pos
	if max < 0 {
		max = 0
	}
	return
}

var (
	ErrFileTooLarge    = errors.New("file too large for cache")
	ErrFileDisappeared = errors.New("file disappeared")
)

func (me *File) checkGone() {
	if me.gone {
		return
	}
	ffi, _ := me.Stat()
	fsfi, _ := os.Stat(me.c.realpath(me.path))
	me.gone = !os.SameFile(ffi, fsfi)
}

func (me *File) goneErr() error {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.checkGone()
	if me.gone {
		me.f.Close()
		return ErrFileDisappeared
	}
	return nil
}

func (me *File) Write(b []byte) (n int, err error) {
	err = me.goneErr()
	if err != nil {
		return
	}
	n, err = me.f.Write(b)
	me.c.mu.Lock()
	me.c.statItem(me.path, time.Now())
	me.c.trimToCapacity()
	me.c.mu.Unlock()
	if err == nil {
		err = me.goneErr()
	}
	return
}

func (me *File) WriteAt(b []byte, off int64) (n int, err error) {
	err = me.goneErr()
	if err != nil {
		return
	}
	n, err = me.f.WriteAt(b, off)
	me.c.mu.Lock()
	me.c.statItem(me.path, time.Now())
	me.c.trimToCapacity()
	me.c.mu.Unlock()
	if err == nil {
		err = me.goneErr()
	}
	return
}

func (me *File) Close() error {
	return me.f.Close()
}

func (me *File) Stat() (os.FileInfo, error) {
	return me.f.Stat()
}

func (me *File) Read(b []byte) (n int, err error) {
	err = me.goneErr()
	if err != nil {
		return
	}
	defer func() {
		me.c.mu.Lock()
		defer me.c.mu.Unlock()
		me.c.statItem(me.path, time.Now())
	}()
	return me.f.Read(b)
}

func (me *File) ReadAt(b []byte, off int64) (n int, err error) {
	err = me.goneErr()
	if err != nil {
		return
	}
	defer func() {
		me.c.mu.Lock()
		defer me.c.mu.Unlock()
		me.c.statItem(me.path, time.Now())
	}()
	return me.f.ReadAt(b, off)
}
