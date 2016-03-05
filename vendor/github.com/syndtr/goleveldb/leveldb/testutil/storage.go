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
	"math/rand"
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
	storageUseFS  = true
	storageKeepFS = false
	storageNum    int
)

type StorageMode int

const (
	ModeOpen StorageMode = 1 << iota
	ModeCreate
	ModeRemove
	ModeRename
	ModeRead
	ModeWrite
	ModeSync
	ModeClose
)

const (
	modeOpen = iota
	modeCreate
	modeRemove
	modeRename
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
	case ModeRename:
		x = modeRename
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
	case m&ModeRename != 0:
		add(modeRename)
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

func packFile(fd storage.FileDesc) uint64 {
	if fd.Num>>(63-typeCount) != 0 {
		panic("overflow")
	}
	return uint64(fd.Num<<typeCount) | uint64(fd.Type)
}

func unpackFile(x uint64) storage.FileDesc {
	return storage.FileDesc{storage.FileType(x) & storage.TypeAll, int64(x >> typeCount)}
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
	s  *Storage
	fd storage.FileDesc
	storage.Reader
}

func (r *reader) Read(p []byte) (n int, err error) {
	err = r.s.emulateError(ModeRead, r.fd.Type)
	if err == nil {
		r.s.stall(ModeRead, r.fd.Type)
		n, err = r.Reader.Read(p)
	}
	r.s.count(ModeRead, r.fd.Type, n)
	if err != nil && err != io.EOF {
		r.s.logI("read error, fd=%s n=%d err=%v", r.fd, n, err)
	}
	return
}

func (r *reader) ReadAt(p []byte, off int64) (n int, err error) {
	err = r.s.emulateError(ModeRead, r.fd.Type)
	if err == nil {
		r.s.stall(ModeRead, r.fd.Type)
		n, err = r.Reader.ReadAt(p, off)
	}
	r.s.count(ModeRead, r.fd.Type, n)
	if err != nil && err != io.EOF {
		r.s.logI("readAt error, fd=%s offset=%d n=%d err=%v", r.fd, off, n, err)
	}
	return
}

func (r *reader) Close() (err error) {
	return r.s.fileClose(r.fd, r.Reader)
}

type writer struct {
	s  *Storage
	fd storage.FileDesc
	storage.Writer
}

func (w *writer) Write(p []byte) (n int, err error) {
	err = w.s.emulateError(ModeWrite, w.fd.Type)
	if err == nil {
		w.s.stall(ModeWrite, w.fd.Type)
		n, err = w.Writer.Write(p)
	}
	w.s.count(ModeWrite, w.fd.Type, n)
	if err != nil && err != io.EOF {
		w.s.logI("write error, fd=%s n=%d err=%v", w.fd, n, err)
	}
	return
}

func (w *writer) Sync() (err error) {
	err = w.s.emulateError(ModeSync, w.fd.Type)
	if err == nil {
		w.s.stall(ModeSync, w.fd.Type)
		err = w.Writer.Sync()
	}
	w.s.count(ModeSync, w.fd.Type, 0)
	if err != nil {
		w.s.logI("sync error, fd=%s err=%v", w.fd, err)
	}
	return
}

func (w *writer) Close() (err error) {
	return w.s.fileClose(w.fd, w.Writer)
}

type Storage struct {
	storage.Storage
	path    string
	onClose func() (preserve bool, err error)
	onLog   func(str string)

	lmu sync.Mutex
	lb  bytes.Buffer

	mu   sync.Mutex
	rand *rand.Rand
	// Open files, true=writer, false=reader
	opens                   map[uint64]bool
	counters                [flattenCount]int
	bytesCounter            [flattenCount]int64
	emulatedError           [flattenCount]error
	emulatedErrorOnce       [flattenCount]bool
	emulatedRandomError     [flattenCount]error
	emulatedRandomErrorProb [flattenCount]float64
	stallCond               sync.Cond
	stalled                 [flattenCount]bool
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
	if s.onLog != nil {
		s.onLog(s.lb.String())
		s.lb.Reset()
	} else {
		s.lb.WriteByte('\n')
	}
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

func (s *Storage) OnLog(onLog func(log string)) {
	s.lmu.Lock()
	s.onLog = onLog
	if s.lb.Len() != 0 {
		log := s.lb.String()
		s.onLog(log[:len(log)-1])
		s.lb.Reset()
	}
	s.lmu.Unlock()
}

func (s *Storage) Log(str string) {
	s.log(1, "Log: "+str)
	s.Storage.Log(str)
}

func (s *Storage) Lock() (l storage.Lock, err error) {
	l, err = s.Storage.Lock()
	if err != nil {
		s.logI("storage locking failed, err=%v", err)
	} else {
		s.logI("storage locked")
		l = storageLock{s, l}
	}
	return
}

func (s *Storage) List(t storage.FileType) (fds []storage.FileDesc, err error) {
	fds, err = s.Storage.List(t)
	if err != nil {
		s.logI("list failed, err=%v", err)
		return
	}
	s.logI("list, type=0x%x count=%d", int(t), len(fds))
	return
}

func (s *Storage) GetMeta() (fd storage.FileDesc, err error) {
	fd, err = s.Storage.GetMeta()
	if err != nil {
		if !os.IsNotExist(err) {
			s.logI("get meta failed, err=%v", err)
		}
		return
	}
	s.logI("get meta, fd=%s", fd)
	return
}

func (s *Storage) SetMeta(fd storage.FileDesc) error {
	ExpectWithOffset(1, fd.Type).To(Equal(storage.TypeManifest))
	err := s.Storage.SetMeta(fd)
	if err != nil {
		s.logI("set meta failed, fd=%s err=%v", fd, err)
	} else {
		s.logI("set meta, fd=%s", fd)
	}
	return err
}

func (s *Storage) fileClose(fd storage.FileDesc, closer io.Closer) (err error) {
	err = s.emulateError(ModeClose, fd.Type)
	if err == nil {
		s.stall(ModeClose, fd.Type)
	}
	x := packFile(fd)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		ExpectWithOffset(2, s.opens).To(HaveKey(x), "File closed, fd=%s", fd)
		err = closer.Close()
	}
	s.countNB(ModeClose, fd.Type, 0)
	writer := s.opens[x]
	if err != nil {
		s.logISkip(1, "file close failed, fd=%s writer=%v err=%v", fd, writer, err)
	} else {
		s.logISkip(1, "file closed, fd=%s writer=%v", fd, writer)
		delete(s.opens, x)
	}
	return
}

func (s *Storage) assertOpen(fd storage.FileDesc) {
	x := packFile(fd)
	ExpectWithOffset(2, s.opens).NotTo(HaveKey(x), "File open, fd=%s writer=%v", fd, s.opens[x])
}

func (s *Storage) Open(fd storage.FileDesc) (r storage.Reader, err error) {
	err = s.emulateError(ModeOpen, fd.Type)
	if err == nil {
		s.stall(ModeOpen, fd.Type)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.assertOpen(fd)
		s.countNB(ModeOpen, fd.Type, 0)
		r, err = s.Storage.Open(fd)
	}
	if err != nil {
		s.logI("file open failed, fd=%s err=%v", fd, err)
	} else {
		s.logI("file opened, fd=%s", fd)
		s.opens[packFile(fd)] = false
		r = &reader{s, fd, r}
	}
	return
}

func (s *Storage) Create(fd storage.FileDesc) (w storage.Writer, err error) {
	err = s.emulateError(ModeCreate, fd.Type)
	if err == nil {
		s.stall(ModeCreate, fd.Type)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.assertOpen(fd)
		s.countNB(ModeCreate, fd.Type, 0)
		w, err = s.Storage.Create(fd)
	}
	if err != nil {
		s.logI("file create failed, fd=%s err=%v", fd, err)
	} else {
		s.logI("file created, fd=%s", fd)
		s.opens[packFile(fd)] = true
		w = &writer{s, fd, w}
	}
	return
}

func (s *Storage) Remove(fd storage.FileDesc) (err error) {
	err = s.emulateError(ModeRemove, fd.Type)
	if err == nil {
		s.stall(ModeRemove, fd.Type)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.assertOpen(fd)
		s.countNB(ModeRemove, fd.Type, 0)
		err = s.Storage.Remove(fd)
	}
	if err != nil {
		s.logI("file remove failed, fd=%s err=%v", fd, err)
	} else {
		s.logI("file removed, fd=%s", fd)
	}
	return
}

func (s *Storage) ForceRemove(fd storage.FileDesc) (err error) {
	s.countNB(ModeRemove, fd.Type, 0)
	if err = s.Storage.Remove(fd); err != nil {
		s.logI("file remove failed (forced), fd=%s err=%v", fd, err)
	} else {
		s.logI("file removed (forced), fd=%s", fd)
	}
	return
}

func (s *Storage) Rename(oldfd, newfd storage.FileDesc) (err error) {
	err = s.emulateError(ModeRename, oldfd.Type)
	if err == nil {
		s.stall(ModeRename, oldfd.Type)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.assertOpen(oldfd)
		s.assertOpen(newfd)
		s.countNB(ModeRename, oldfd.Type, 0)
		err = s.Storage.Rename(oldfd, newfd)
	}
	if err != nil {
		s.logI("file rename failed, oldfd=%s newfd=%s err=%v", oldfd, newfd, err)
	} else {
		s.logI("file renamed, oldfd=%s newfd=%s", oldfd, newfd)
	}
	return
}

func (s *Storage) ForceRename(oldfd, newfd storage.FileDesc) (err error) {
	s.countNB(ModeRename, oldfd.Type, 0)
	if err = s.Storage.Rename(oldfd, newfd); err != nil {
		s.logI("file rename failed (forced), oldfd=%s newfd=%s err=%v", oldfd, newfd, err)
	} else {
		s.logI("file renamed (forced), oldfd=%s newfd=%s", oldfd, newfd)
	}
	return
}

func (s *Storage) openFiles() string {
	out := "Open files:"
	for x, writer := range s.opens {
		fd := unpackFile(x)
		out += fmt.Sprintf("\n · fd=%s writer=%v", fd, writer)
	}
	return out
}

func (s *Storage) CloseCheck() {
	s.mu.Lock()
	defer s.mu.Unlock()
	ExpectWithOffset(1, s.opens).To(BeEmpty(), s.openFiles())
}

func (s *Storage) OnClose(onClose func() (preserve bool, err error)) {
	s.mu.Lock()
	s.onClose = onClose
	s.mu.Unlock()
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
	var preserve bool
	if s.onClose != nil {
		var err0 error
		if preserve, err0 = s.onClose(); err0 != nil {
			s.logI("onClose error, err=%v", err0)
		}
	}
	if s.path != "" {
		if storageKeepFS || preserve {
			s.logI("storage is preserved, path=%v", s.path)
		} else {
			if err1 := os.RemoveAll(s.path); err1 != nil {
				s.logI("cannot remove storage, err=%v", err1)
			} else {
				s.logI("storage has been removed")
			}
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
	x := flattenType(m, t)
	if err := s.emulatedError[x]; err != nil {
		if s.emulatedErrorOnce[x] {
			s.emulatedError[x] = nil
		}
		return emulatedError{err}
	}
	if err := s.emulatedRandomError[x]; err != nil && s.rand.Float64() < s.emulatedRandomErrorProb[x] {
		return emulatedError{err}
	}
	return nil
}

func (s *Storage) EmulateError(m StorageMode, t storage.FileType, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range listFlattenType(m, t) {
		s.emulatedError[x] = err
		s.emulatedErrorOnce[x] = false
	}
}

func (s *Storage) EmulateErrorOnce(m StorageMode, t storage.FileType, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range listFlattenType(m, t) {
		s.emulatedError[x] = err
		s.emulatedErrorOnce[x] = true
	}
}

func (s *Storage) EmulateRandomError(m StorageMode, t storage.FileType, prob float64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range listFlattenType(m, t) {
		s.emulatedRandomError[x] = err
		s.emulatedRandomErrorProb[x] = prob
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
	var (
		stor storage.Storage
		path string
	)
	if storageUseFS {
		for {
			storageMu.Lock()
			num := storageNum
			storageNum++
			storageMu.Unlock()
			path = filepath.Join(os.TempDir(), fmt.Sprintf("goleveldb-test%d0%d0%d", os.Getuid(), os.Getpid(), num))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				stor, err = storage.OpenFile(path, false)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), "creating storage at %s", path)
				break
			}
		}
	} else {
		stor = storage.NewMemStorage()
	}
	s := &Storage{
		Storage: stor,
		path:    path,
		rand:    NewRand(),
		opens:   make(map[uint64]bool),
	}
	s.stallCond.L = &s.mu
	if s.path != "" {
		s.logI("using FS storage")
		s.logI("storage path: %s", s.path)
	} else {
		s.logI("using MEM storage")
	}
	return s
}
