// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"bytes"
	"os"
	"sync"

	"github.com/syndtr/goleveldb/leveldb/util"
)

const typeShift = 3

type memStorageLock struct {
	ms *memStorage
}

func (lock *memStorageLock) Release() {
	ms := lock.ms
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.slock == lock {
		ms.slock = nil
	}
	return
}

// memStorage is a memory-backed storage.
type memStorage struct {
	mu       sync.Mutex
	slock    *memStorageLock
	files    map[uint64]*memFile
	manifest *memFilePtr
}

// NewMemStorage returns a new memory-backed storage implementation.
func NewMemStorage() Storage {
	return &memStorage{
		files: make(map[uint64]*memFile),
	}
}

func (ms *memStorage) Lock() (util.Releaser, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.slock != nil {
		return nil, ErrLocked
	}
	ms.slock = &memStorageLock{ms: ms}
	return ms.slock, nil
}

func (*memStorage) Log(str string) {}

func (ms *memStorage) GetFile(num uint64, t FileType) File {
	return &memFilePtr{ms: ms, num: num, t: t}
}

func (ms *memStorage) GetFiles(t FileType) ([]File, error) {
	ms.mu.Lock()
	var ff []File
	for x, _ := range ms.files {
		num, mt := x>>typeShift, FileType(x)&TypeAll
		if mt&t == 0 {
			continue
		}
		ff = append(ff, &memFilePtr{ms: ms, num: num, t: mt})
	}
	ms.mu.Unlock()
	return ff, nil
}

func (ms *memStorage) GetManifest() (File, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.manifest == nil {
		return nil, os.ErrNotExist
	}
	return ms.manifest, nil
}

func (ms *memStorage) SetManifest(f File) error {
	fm, ok := f.(*memFilePtr)
	if !ok || fm.t != TypeManifest {
		return ErrInvalidFile
	}
	ms.mu.Lock()
	ms.manifest = fm
	ms.mu.Unlock()
	return nil
}

func (*memStorage) Close() error { return nil }

type memReader struct {
	*bytes.Reader
	m *memFile
}

func (mr *memReader) Close() error {
	return mr.m.Close()
}

type memFile struct {
	bytes.Buffer
	ms   *memStorage
	open bool
}

func (*memFile) Sync() error { return nil }
func (m *memFile) Close() error {
	m.ms.mu.Lock()
	m.open = false
	m.ms.mu.Unlock()
	return nil
}

type memFilePtr struct {
	ms  *memStorage
	num uint64
	t   FileType
}

func (p *memFilePtr) x() uint64 {
	return p.Num()<<typeShift | uint64(p.Type())
}

func (p *memFilePtr) Open() (Reader, error) {
	ms := p.ms
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if m, exist := ms.files[p.x()]; exist {
		if m.open {
			return nil, errFileOpen
		}
		m.open = true
		return &memReader{Reader: bytes.NewReader(m.Bytes()), m: m}, nil
	}
	return nil, os.ErrNotExist
}

func (p *memFilePtr) Create() (Writer, error) {
	ms := p.ms
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m, exist := ms.files[p.x()]
	if exist {
		if m.open {
			return nil, errFileOpen
		}
		m.Reset()
	} else {
		m = &memFile{ms: ms}
		ms.files[p.x()] = m
	}
	m.open = true
	return m, nil
}

func (p *memFilePtr) Replace(newfile File) error {
	p1, ok := newfile.(*memFilePtr)
	if !ok {
		return ErrInvalidFile
	}
	ms := p.ms
	ms.mu.Lock()
	defer ms.mu.Unlock()
	m1, exist := ms.files[p1.x()]
	if !exist {
		return os.ErrNotExist
	}
	m0, exist := ms.files[p.x()]
	if (exist && m0.open) || m1.open {
		return errFileOpen
	}
	delete(ms.files, p1.x())
	ms.files[p.x()] = m1
	return nil
}

func (p *memFilePtr) Type() FileType {
	return p.t
}

func (p *memFilePtr) Num() uint64 {
	return p.num
}

func (p *memFilePtr) Remove() error {
	ms := p.ms
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, exist := ms.files[p.x()]; exist {
		delete(ms.files, p.x())
		return nil
	}
	return os.ErrNotExist
}
