// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	. "github.com/onsi/gomega"

	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	storageMu     sync.Mutex
	storageUseFS  bool = true
	storageKeepFS bool = false
	storageNum    int
)

type StorageMode int

const (
	ModeOpen StorageMode = 1 << iota
	ModeCreate
	ModeRemove
	ModeRead
	ModeWrite
	ModeSync
	ModeClose
)

const (
	modeOpen = iota
	modeCreate
	modeRemove
	modeRead
	modeWrite
	modeSync
	modeClose

	modeCount
)

const (
	typeManifest = iota
	typeJournal
	typeTable
	typeTemp

	typeCount
)

const flattenCount = modeCount * typeCount

func flattenType(m StorageMode, t storage.FileType) int {
	var x int
	switch m {
	case ModeOpen:
		x = modeOpen
	case ModeCreate:
		x = modeCreate
	case ModeRemove:
		x = modeRemove
	case ModeRead:
		x = modeRead
	case ModeWrite:
		x = modeWrite
	case ModeSync:
		x = modeSync
	case ModeClose:
		x = modeClose
	default:
		panic("invalid storage mode")
	}
	x *= typeCount
	switch t {
	case storage.TypeManifest:
		return x + typeManifest
	case storage.TypeJournal:
		return x + typeJournal
	case storage.TypeTable:
		return x + typeTable
	case storage.TypeTemp:
		return x + typeTemp
	default:
		panic("invalid file type")
	}
}

func listFlattenType(m StorageMode, t storage.FileType) []int {
	ret := make([]int, 0, flattenCount)
	add := func(x int) {
		x *= typeCount
		switch {
		case t&storage.TypeManifest != 0:
			ret = append(ret, x+typeManifest)
		case t&storage.TypeJournal != 0:
			ret = append(ret, x+typeJournal)
		case t&storage.TypeTable != 0:
			ret = append(ret, x+typeTable)
		case t&storage.TypeTemp != 0:
			ret = append(ret, x+typeTemp)
		}
	}
	switch {
	case m&ModeOpen != 0:
		add(modeOpen)
	case m&ModeCreate != 0:
		add(modeCreate)
	case m&ModeRemove != 0:
		add(modeRemove)
	case m&ModeRead != 0:
		add(modeRead)
	case m&ModeWrite != 0:
		add(modeWrite)
	case m&ModeSync != 0:
		add(modeSync)
	case m&ModeClose != 0:
		add(modeClose)
	}
	return ret
}

func packFile(num uint64, t storage.FileType) uint64 {
	if num>>(64-typeCount) != 0 {
		panic("overflow")
	}
	return num<<typeCount | uint64(t)
}

func unpackFile(x uint64) (uint64, storage.FileType) {
	return x >> typeCount, storage.FileType(x) & storage.TypeAll
}

type emulatedError struct {
	err error
}

func (err emulatedError) Error() string {
	return fmt.Sprintf("emulated storage error: %v", err.err)
}

type storageLock struct {
	s *Storage
	r util.Releaser
}

func (l storageLock) Release() {
	l.r.Release()
	l.s.logI("storage lock released")
}

type reader struct {
	f *file
	storage.Reader
}

func (r *reader) Read(p []byte) (n int, err error) {
	err = r.f.s.emulateError(ModeRead, r.f.Type())
	if err == nil {
		r.f.s.stall(ModeRead, r.f.Type())
		n, err = r.Reader.Read(p)
	}
	r.f.s.count(ModeRead, r.f.Type(), n)
	if err != nil && err != io.EOF {
		r.f.s.logI("read error, num=%d type=%v n=%d err=%v", r.f.Num(), r.f.Type(), n, err)
	}
	return
}

func (r *reader) ReadAt(p []byte, off int64) (n int, err error) {
	err = r.f.s.emulateError(ModeRead, r.f.Type())
	if err == nil {
		r.f.s.stall(ModeRead, r.f.Type())
		n, err = r.Reader.ReadAt(p, off)
	}
	r.f.s.count(ModeRead, r.f.Type(), n)
	if err != nil && err != io.EOF {
		r.f.s.logI("readAt error, num=%d type=%v offset=%d n=%d err=%v", r.f.Num(), r.f.Type(), off, n, err)
	}
	return
}

func (r *reader) Close() (err error) {
	return r.f.doClose(r.Reader)
}

type writer struct {
	f *file
	storage.Writer
}

func (w *writer) Write(p []byte) (n int, err error) {
	err = w.f.s.emulateError(ModeWrite, w.f.Type())
	if err == nil {
		w.f.s.stall(ModeWrite, w.f.Type())
		n, err = w.Writer.Write(p)
	}
	w.f.s.count(ModeWrite, w.f.Type(), n)
	if err != nil && err != io.EOF {
		w.f.s.logI("write error, num=%d type=%v n=%d err=%v", w.f.Num(), w.f.Type(), n, err)
	}
	return
}

func (w *writer) Sync() (err error) {
	err = w.f.s.emulateError(ModeSync, w.f.Type())
	if err == nil {
		w.f.s.stall(ModeSync, w.f.Type())
		err = w.Writer.Sync()
	}
	w.f.s.count(ModeSync, w.f.Type(), 0)
	if err != nil {
		w.f.s.logI("sync error, num=%d type=%v err=%v", w.f.Num(), w.f.Type(), err)
	}
	return
}

func (w *writer) Close() (err error) {
	return w.f.doClose(w.Writer)
}

type file struct {
	s *Storage
	storage.File
}

func (f *file) pack() uint64 {
	return packFile(f.Num(), f.Type())
}

func (f *file) assertOpen() {
	ExpectWithOffset(2, f.s.opens).NotTo(HaveKey(f.pack()), "File open, num=%d type=%v writer=%v", f.Num(), f.Type(), f.s.opens[f.pack()])
}

func (f *file) doClose(closer io.Closer) (err error) {
	err = f.s.emulateError(ModeClose, f.Type())
	if err == nil {
		f.s.stall(ModeClose, f.Type())
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if err == nil {
		ExpectWithOffset(2, f.s.opens).To(HaveKey(f.pack()), "File closed, num=%d type=%v", f.Num(), f.Type())
		err = closer.Close()
	}
	f.s.countNB(ModeClose, f.Type(), 0)
	writer := f.s.opens[f.pack()]
	if err != nil {
		f.s.logISkip(1, "file close failed, num=%d type=%v writer=%v err=%v", f.Num(), f.Type(), writer, err)
	} else {
		f.s.logISkip(1, "file closed, num=%d type=%v writer=%v", f.Num(), f.Type(), writer)
		delete(f.s.opens, f.pack())
	}
	return
}

func (f *file) Open() (r storage.Reader, err error) {
	err = f.s.emulateError(ModeOpen, f.Type())
	if err == nil {
		f.s.stall(ModeOpen, f.Type())
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if err == nil {
		f.assertOpen()
		f.s.countNB(ModeOpen, f.Type(), 0)
		r, err = f.File.Open()
	}
	if err != nil {
		f.s.logI("file open failed, num=%d type=%v err=%v", f.Num(), f.Type(), err)
	} else {
		f.s.logI("file opened, num=%d type=%v", f.Num(), f.Type())
		f.s.opens[f.pack()] = false
		r = &reader{f, r}
	}
	return
}

func (f *file) Create() (w storage.Writer, err error) {
	err = f.s.emulateError(ModeCreate, f.Type())
	if err == nil {
		f.s.stall(ModeCreate, f.Type())
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if err == nil {
		f.assertOpen()
		f.s.countNB(ModeCreate, f.Type(), 0)
		w, err = f.File.Create()
	}
	if err != nil {
		f.s.logI("file create failed, num=%d type=%v err=%v", f.Num(), f.Type(), err)
	} else {
		f.s.logI("file created, num=%d type=%v", f.Num(), f.Type())
		f.s.opens[f.pack()] = true
		w = &writer{f, w}
	}
	return
}

func (f *file) Remove() (err error) {
	err = f.s.emulateError(ModeRemove, f.Type())
	if err == nil {
		f.s.stall(ModeRemove, f.Type())
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if err == nil {
		f.assertOpen()
		f.s.countNB(ModeRemove, f.Type(), 0)
		err = f.File.Remove()
	}
	if err != nil {
		f.s.logI("file remove failed, num=%d type=%v err=%v", f.Num(), f.Type(), err)
	} else {
		f.s.logI("file removed, num=%d type=%v", f.Num(), f.Type())
	}
	return
}

type Storage struct {
	storage.Storage
	closeFn func() error

	lmu sync.Mutex
	lb  bytes.Buffer

	mu sync.Mutex
	// Open files, true=writer, false=reader
	opens         map[uint64]bool
	counters      [flattenCount]int
	bytesCounter  [flattenCount]int64
	emulatedError [flattenCount]error
	stallCond     sync.Cond
	stalled       [flattenCount]bool
}

func (s *Storage) log(skip int, str string) {
	s.lmu.Lock()
	defer s.lmu.Unlock()
	_, file, line, ok := runtime.Caller(skip + 2)
	if ok {
		// Truncate file name at last file name separator.
		if index := strings.LastIndex(file, "/"); index >= 0 {
			file = file[index+1:]
		} else if index = strings.LastIndex(file, "\\"); index >= 0 {
			file = file[index+1:]
		}
	} else {
		file = "???"
		line = 1
	}
	fmt.Fprintf(&s.lb, "%s:%d: ", file, line)
	lines := strings.Split(str, "\n")
	if l := len(lines); l > 1 && lines[l-1] == "" {
		lines = lines[:l-1]
	}
	for i, line := range lines {
		if i > 0 {
			s.lb.WriteString("\n\t")
		}
		s.lb.WriteString(line)
	}
	s.lb.WriteByte('\n')
}

func (s *Storage) logISkip(skip int, format string, args ...interface{}) {
	pc, _, _, ok := runtime.Caller(skip + 1)
	if ok {
		if f := runtime.FuncForPC(pc); f != nil {
			fname := f.Name()
			if index := strings.LastIndex(fname, "."); index >= 0 {
				fname = fname[index+1:]
			}
			format = fname + ": " + format
		}
	}
	s.log(skip+1, fmt.Sprintf(format, args...))
}

func (s *Storage) logI(format string, args ...interface{}) {
	s.logISkip(1, format, args...)
}

func (s *Storage) Log(str string) {
	s.log(1, "Log: "+str)
	s.Storage.Log(str)
}

func (s *Storage) Lock() (r util.Releaser, err error) {
	r, err = s.Storage.Lock()
	if err != nil {
		s.logI("storage locking failed, err=%v", err)
	} else {
		s.logI("storage locked")
		r = storageLock{s, r}
	}
	return
}

func (s *Storage) GetFile(num uint64, t storage.FileType) storage.File {
	return &file{s, s.Storage.GetFile(num, t)}
}

func (s *Storage) GetFiles(t storage.FileType) (files []storage.File, err error) {
	rfiles, err := s.Storage.GetFiles(t)
	if err != nil {
		s.logI("get files failed, err=%v", err)
		return
	}
	files = make([]storage.File, len(rfiles))
	for i, f := range rfiles {
		files[i] = &file{s, f}
	}
	s.logI("get files, type=0x%x count=%d", int(t), len(files))
	return
}

func (s *Storage) GetManifest() (f storage.File, err error) {
	manifest, err := s.Storage.GetManifest()
	if err != nil {
		if !os.IsNotExist(err) {
			s.logI("get manifest failed, err=%v", err)
		}
		return
	}
	s.logI("get manifest, num=%d", manifest.Num())
	return &file{s, manifest}, nil
}

func (s *Storage) SetManifest(f storage.File) error {
	f_, ok := f.(*file)
	ExpectWithOffset(1, ok).To(BeTrue())
	ExpectWithOffset(1, f_.Type()).To(Equal(storage.TypeManifest))
	err := s.Storage.SetManifest(f_.File)
	if err != nil {
		s.logI("set manifest failed, err=%v", err)
	} else {
		s.logI("set manifest, num=%d", f_.Num())
	}
	return err
}

func (s *Storage) openFiles() string {
	out := "Open files:"
	for x, writer := range s.opens {
		num, t := unpackFile(x)
		out += fmt.Sprintf("\n · num=%d type=%v writer=%v", num, t, writer)
	}
	return out
}

func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ExpectWithOffset(1, s.opens).To(BeEmpty(), s.openFiles())
	err := s.Storage.Close()
	if err != nil {
		s.logI("storage closing failed, err=%v", err)
	} else {
		s.logI("storage closed")
	}
	if s.closeFn != nil {
		if err1 := s.closeFn(); err1 != nil {
			s.logI("close func error, err=%v", err1)
		}
	}
	return err
}

func (s *Storage) countNB(m StorageMode, t storage.FileType, n int) {
	s.counters[flattenType(m, t)]++
	s.bytesCounter[flattenType(m, t)] += int64(n)
}

func (s *Storage) count(m StorageMode, t storage.FileType, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countNB(m, t, n)
}

func (s *Storage) ResetCounter(m StorageMode, t storage.FileType) {
	for _, x := range listFlattenType(m, t) {
		s.counters[x] = 0
		s.bytesCounter[x] = 0
	}
}

func (s *Storage) Counter(m StorageMode, t storage.FileType) (count int, bytes int64) {
	for _, x := range listFlattenType(m, t) {
		count += s.counters[x]
		bytes += s.bytesCounter[x]
	}
	return
}

func (s *Storage) emulateError(m StorageMode, t storage.FileType) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.emulatedError[flattenType(m, t)]
	if err != nil {
		return emulatedError{err}
	}
	return nil
}

func (s *Storage) EmulateError(m StorageMode, t storage.FileType, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range listFlattenType(m, t) {
		s.emulatedError[x] = err
	}
}

func (s *Storage) stall(m StorageMode, t storage.FileType) {
	x := flattenType(m, t)
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.stalled[x] {
		s.stallCond.Wait()
	}
}

func (s *Storage) Stall(m StorageMode, t storage.FileType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range listFlattenType(m, t) {
		s.stalled[x] = true
	}
}

func (s *Storage) Release(m StorageMode, t storage.FileType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range listFlattenType(m, t) {
		s.stalled[x] = false
	}
	s.stallCond.Broadcast()
}

func NewStorage() *Storage {
	var stor storage.Storage
	var closeFn func() error
	if storageUseFS {
		for {
			storageMu.Lock()
			num := storageNum
			storageNum++
			storageMu.Unlock()
			path := filepath.Join(os.TempDir(), fmt.Sprintf("goleveldb-test%d0%d0%d", os.Getuid(), os.Getpid(), num))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				stor, err = storage.OpenFile(path)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), "creating storage at %s", path)
				closeFn = func() error {
					if storageKeepFS {
						return nil
					}
					return os.RemoveAll(path)
				}
				break
			}
		}
	} else {
		stor = storage.NewMemStorage()
	}
	s := &Storage{
		Storage: stor,
		closeFn: closeFn,
		opens:   make(map[uint64]bool),
	}
	s.stallCond.L = &s.mu
	return s
}
