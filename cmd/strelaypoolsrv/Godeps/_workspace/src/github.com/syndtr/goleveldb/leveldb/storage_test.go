// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENE file.

package leveldb

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const typeShift = 4

var (
	tsErrInvalidFile = errors.New("leveldb.testStorage: invalid file for argument")
	tsErrFileOpen    = errors.New("leveldb.testStorage: file still open")
)

var (
	tsFSEnv   = os.Getenv("GOLEVELDB_USEFS")
	tsTempdir = os.Getenv("GOLEVELDB_TEMPDIR")
	tsKeepFS  = tsFSEnv == "2"
	tsFS      = tsKeepFS || tsFSEnv == "" || tsFSEnv == "1"
	tsMU      = &sync.Mutex{}
	tsNum     = 0
)

type tsOp uint

const (
	tsOpOpen tsOp = iota
	tsOpCreate
	tsOpRead
	tsOpReadAt
	tsOpWrite
	tsOpSync

	tsOpNum
)

type tsLock struct {
	ts *testStorage
	r  util.Releaser
}

func (l tsLock) Release() {
	l.r.Release()
	l.ts.t.Log("I: storage lock released")
}

type tsReader struct {
	tf tsFile
	storage.Reader
}

func (tr tsReader) Read(b []byte) (n int, err error) {
	ts := tr.tf.ts
	ts.countRead(tr.tf.Type())
	if tr.tf.shouldErrLocked(tsOpRead) {
		return 0, errors.New("leveldb.testStorage: emulated read error")
	}
	n, err = tr.Reader.Read(b)
	if err != nil && err != io.EOF {
		ts.t.Errorf("E: read error, num=%d type=%v n=%d: %v", tr.tf.Num(), tr.tf.Type(), n, err)
	}
	return
}

func (tr tsReader) ReadAt(b []byte, off int64) (n int, err error) {
	ts := tr.tf.ts
	ts.countRead(tr.tf.Type())
	if tr.tf.shouldErrLocked(tsOpReadAt) {
		return 0, errors.New("leveldb.testStorage: emulated readAt error")
	}
	n, err = tr.Reader.ReadAt(b, off)
	if err != nil && err != io.EOF {
		ts.t.Errorf("E: readAt error, num=%d type=%v off=%d n=%d: %v", tr.tf.Num(), tr.tf.Type(), off, n, err)
	}
	return
}

func (tr tsReader) Close() (err error) {
	err = tr.Reader.Close()
	tr.tf.close("reader", err)
	return
}

type tsWriter struct {
	tf tsFile
	storage.Writer
}

func (tw tsWriter) Write(b []byte) (n int, err error) {
	if tw.tf.shouldErrLocked(tsOpWrite) {
		return 0, errors.New("leveldb.testStorage: emulated write error")
	}
	n, err = tw.Writer.Write(b)
	if err != nil {
		tw.tf.ts.t.Errorf("E: write error, num=%d type=%v n=%d: %v", tw.tf.Num(), tw.tf.Type(), n, err)
	}
	return
}

func (tw tsWriter) Sync() (err error) {
	ts := tw.tf.ts
	ts.mu.Lock()
	for ts.emuDelaySync&tw.tf.Type() != 0 {
		ts.cond.Wait()
	}
	ts.mu.Unlock()
	if tw.tf.shouldErrLocked(tsOpSync) {
		return errors.New("leveldb.testStorage: emulated sync error")
	}
	err = tw.Writer.Sync()
	if err != nil {
		tw.tf.ts.t.Errorf("E: sync error, num=%d type=%v: %v", tw.tf.Num(), tw.tf.Type(), err)
	}
	return
}

func (tw tsWriter) Close() (err error) {
	err = tw.Writer.Close()
	tw.tf.close("writer", err)
	return
}

type tsFile struct {
	ts *testStorage
	storage.File
}

func (tf tsFile) x() uint64 {
	return tf.Num()<<typeShift | uint64(tf.Type())
}

func (tf tsFile) shouldErr(op tsOp) bool {
	return tf.ts.shouldErr(tf, op)
}

func (tf tsFile) shouldErrLocked(op tsOp) bool {
	tf.ts.mu.Lock()
	defer tf.ts.mu.Unlock()
	return tf.shouldErr(op)
}

func (tf tsFile) checkOpen(m string) error {
	ts := tf.ts
	if writer, ok := ts.opens[tf.x()]; ok {
		if writer {
			ts.t.Errorf("E: cannot %s file, num=%d type=%v: a writer still open", m, tf.Num(), tf.Type())
		} else {
			ts.t.Errorf("E: cannot %s file, num=%d type=%v: a reader still open", m, tf.Num(), tf.Type())
		}
		return tsErrFileOpen
	}
	return nil
}

func (tf tsFile) close(m string, err error) {
	ts := tf.ts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if _, ok := ts.opens[tf.x()]; !ok {
		ts.t.Errorf("E: %s: redudant file closing, num=%d type=%v", m, tf.Num(), tf.Type())
	} else if err == nil {
		ts.t.Logf("I: %s: file closed, num=%d type=%v", m, tf.Num(), tf.Type())
	}
	delete(ts.opens, tf.x())
	if err != nil {
		ts.t.Errorf("E: %s: cannot close file, num=%d type=%v: %v", m, tf.Num(), tf.Type(), err)
	}
}

func (tf tsFile) Open() (r storage.Reader, err error) {
	ts := tf.ts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	err = tf.checkOpen("open")
	if err != nil {
		return
	}
	if tf.shouldErr(tsOpOpen) {
		err = errors.New("leveldb.testStorage: emulated open error")
		return
	}
	r, err = tf.File.Open()
	if err != nil {
		if ts.ignoreOpenErr&tf.Type() != 0 {
			ts.t.Logf("I: cannot open file, num=%d type=%v: %v (ignored)", tf.Num(), tf.Type(), err)
		} else {
			ts.t.Errorf("E: cannot open file, num=%d type=%v: %v", tf.Num(), tf.Type(), err)
		}
	} else {
		ts.t.Logf("I: file opened, num=%d type=%v", tf.Num(), tf.Type())
		ts.opens[tf.x()] = false
		r = tsReader{tf, r}
	}
	return
}

func (tf tsFile) Create() (w storage.Writer, err error) {
	ts := tf.ts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	err = tf.checkOpen("create")
	if err != nil {
		return
	}
	if tf.shouldErr(tsOpCreate) {
		err = errors.New("leveldb.testStorage: emulated create error")
		return
	}
	w, err = tf.File.Create()
	if err != nil {
		ts.t.Errorf("E: cannot create file, num=%d type=%v: %v", tf.Num(), tf.Type(), err)
	} else {
		ts.t.Logf("I: file created, num=%d type=%v", tf.Num(), tf.Type())
		ts.opens[tf.x()] = true
		w = tsWriter{tf, w}
	}
	return
}

func (tf tsFile) Replace(newfile storage.File) (err error) {
	ts := tf.ts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	err = tf.checkOpen("replace")
	if err != nil {
		return
	}
	err = tf.File.Replace(newfile.(tsFile).File)
	if err != nil {
		ts.t.Errorf("E: cannot replace file, num=%d type=%v: %v", tf.Num(), tf.Type(), err)
	} else {
		ts.t.Logf("I: file replace, num=%d type=%v", tf.Num(), tf.Type())
	}
	return
}

func (tf tsFile) Remove() (err error) {
	ts := tf.ts
	ts.mu.Lock()
	defer ts.mu.Unlock()
	err = tf.checkOpen("remove")
	if err != nil {
		return
	}
	err = tf.File.Remove()
	if err != nil {
		ts.t.Errorf("E: cannot remove file, num=%d type=%v: %v", tf.Num(), tf.Type(), err)
	} else {
		ts.t.Logf("I: file removed, num=%d type=%v", tf.Num(), tf.Type())
	}
	return
}

type testStorage struct {
	t *testing.T
	storage.Storage
	closeFn func() error

	mu   sync.Mutex
	cond sync.Cond
	// Open files, true=writer, false=reader
	opens         map[uint64]bool
	emuDelaySync  storage.FileType
	ignoreOpenErr storage.FileType
	readCnt       uint64
	readCntEn     storage.FileType

	emuErr         [tsOpNum]storage.FileType
	emuErrOnce     [tsOpNum]storage.FileType
	emuRandErr     [tsOpNum]storage.FileType
	emuRandErrProb int
	emuErrOnceMap  map[uint64]uint
	emuRandRand    *rand.Rand
}

func (ts *testStorage) shouldErr(tf tsFile, op tsOp) bool {
	if ts.emuErr[op]&tf.Type() != 0 {
		return true
	} else if ts.emuRandErr[op]&tf.Type() != 0 || ts.emuErrOnce[op]&tf.Type() != 0 {
		sop := uint(1) << op
		eop := ts.emuErrOnceMap[tf.x()]
		if eop&sop == 0 && (ts.emuRandRand.Int()%ts.emuRandErrProb == 0 || ts.emuErrOnce[op]&tf.Type() != 0) {
			ts.emuErrOnceMap[tf.x()] = eop | sop
			ts.t.Logf("I: emulated error: file=%d type=%v op=%v", tf.Num(), tf.Type(), op)
			return true
		}
	}
	return false
}

func (ts *testStorage) SetEmuErr(t storage.FileType, ops ...tsOp) {
	ts.mu.Lock()
	for _, op := range ops {
		ts.emuErr[op] = t
	}
	ts.mu.Unlock()
}

func (ts *testStorage) SetEmuErrOnce(t storage.FileType, ops ...tsOp) {
	ts.mu.Lock()
	for _, op := range ops {
		ts.emuErrOnce[op] = t
	}
	ts.mu.Unlock()
}

func (ts *testStorage) SetEmuRandErr(t storage.FileType, ops ...tsOp) {
	ts.mu.Lock()
	for _, op := range ops {
		ts.emuRandErr[op] = t
	}
	ts.mu.Unlock()
}

func (ts *testStorage) SetEmuRandErrProb(prob int) {
	ts.mu.Lock()
	ts.emuRandErrProb = prob
	ts.mu.Unlock()
}

func (ts *testStorage) DelaySync(t storage.FileType) {
	ts.mu.Lock()
	ts.emuDelaySync |= t
	ts.cond.Broadcast()
	ts.mu.Unlock()
}

func (ts *testStorage) ReleaseSync(t storage.FileType) {
	ts.mu.Lock()
	ts.emuDelaySync &= ^t
	ts.cond.Broadcast()
	ts.mu.Unlock()
}

func (ts *testStorage) ReadCounter() uint64 {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.readCnt
}

func (ts *testStorage) ResetReadCounter() {
	ts.mu.Lock()
	ts.readCnt = 0
	ts.mu.Unlock()
}

func (ts *testStorage) SetReadCounter(t storage.FileType) {
	ts.mu.Lock()
	ts.readCntEn = t
	ts.mu.Unlock()
}

func (ts *testStorage) countRead(t storage.FileType) {
	ts.mu.Lock()
	if ts.readCntEn&t != 0 {
		ts.readCnt++
	}
	ts.mu.Unlock()
}

func (ts *testStorage) SetIgnoreOpenErr(t storage.FileType) {
	ts.ignoreOpenErr = t
}

func (ts *testStorage) Lock() (r util.Releaser, err error) {
	r, err = ts.Storage.Lock()
	if err != nil {
		ts.t.Logf("W: storage locking failed: %v", err)
	} else {
		ts.t.Log("I: storage locked")
		r = tsLock{ts, r}
	}
	return
}

func (ts *testStorage) Log(str string) {
	ts.t.Log("L: " + str)
	ts.Storage.Log(str)
}

func (ts *testStorage) GetFile(num uint64, t storage.FileType) storage.File {
	return tsFile{ts, ts.Storage.GetFile(num, t)}
}

func (ts *testStorage) GetFiles(t storage.FileType) (ff []storage.File, err error) {
	ff0, err := ts.Storage.GetFiles(t)
	if err != nil {
		ts.t.Errorf("E: get files failed: %v", err)
		return
	}
	ff = make([]storage.File, len(ff0))
	for i, f := range ff0 {
		ff[i] = tsFile{ts, f}
	}
	ts.t.Logf("I: get files, type=0x%x count=%d", int(t), len(ff))
	return
}

func (ts *testStorage) GetManifest() (f storage.File, err error) {
	f0, err := ts.Storage.GetManifest()
	if err != nil {
		if !os.IsNotExist(err) {
			ts.t.Errorf("E: get manifest failed: %v", err)
		}
		return
	}
	f = tsFile{ts, f0}
	ts.t.Logf("I: get manifest, num=%d", f.Num())
	return
}

func (ts *testStorage) SetManifest(f storage.File) error {
	tf, ok := f.(tsFile)
	if !ok {
		ts.t.Error("E: set manifest failed: type assertion failed")
		return tsErrInvalidFile
	} else if tf.Type() != storage.TypeManifest {
		ts.t.Errorf("E: set manifest failed: invalid file type: %s", tf.Type())
		return tsErrInvalidFile
	}
	err := ts.Storage.SetManifest(tf.File)
	if err != nil {
		ts.t.Errorf("E: set manifest failed: %v", err)
	} else {
		ts.t.Logf("I: set manifest, num=%d", tf.Num())
	}
	return err
}

func (ts *testStorage) Close() error {
	ts.CloseCheck()
	err := ts.Storage.Close()
	if err != nil {
		ts.t.Errorf("E: closing storage failed: %v", err)
	} else {
		ts.t.Log("I: storage closed")
	}
	if ts.closeFn != nil {
		if err := ts.closeFn(); err != nil {
			ts.t.Errorf("E: close function: %v", err)
		}
	}
	return err
}

func (ts *testStorage) CloseCheck() {
	ts.mu.Lock()
	if len(ts.opens) == 0 {
		ts.t.Log("I: all files are closed")
	} else {
		ts.t.Errorf("E: %d files still open", len(ts.opens))
		for x, writer := range ts.opens {
			num, tt := x>>typeShift, storage.FileType(x)&storage.TypeAll
			ts.t.Errorf("E: * num=%d type=%v writer=%v", num, tt, writer)
		}
	}
	ts.mu.Unlock()
}

func newTestStorage(t *testing.T) *testStorage {
	var stor storage.Storage
	var closeFn func() error
	if tsFS {
		for {
			tsMU.Lock()
			num := tsNum
			tsNum++
			tsMU.Unlock()
			tempdir := tsTempdir
			if tempdir == "" {
				tempdir = os.TempDir()
			}
			path := filepath.Join(tempdir, fmt.Sprintf("goleveldb-test%d0%d0%d", os.Getuid(), os.Getpid(), num))
			if _, err := os.Stat(path); err != nil {
				stor, err = storage.OpenFile(path)
				if err != nil {
					t.Fatalf("F: cannot create storage: %v", err)
				}
				t.Logf("I: storage created: %s", path)
				closeFn = func() error {
					for _, name := range []string{"LOG.old", "LOG"} {
						f, err := os.Open(filepath.Join(path, name))
						if err != nil {
							continue
						}
						if log, err := ioutil.ReadAll(f); err != nil {
							t.Logf("---------------------- %s ----------------------", name)
							t.Logf("cannot read log: %v", err)
							t.Logf("---------------------- %s ----------------------", name)
						} else if len(log) > 0 {
							t.Logf("---------------------- %s ----------------------\n%s", name, string(log))
							t.Logf("---------------------- %s ----------------------", name)
						}
						f.Close()
					}
					if t.Failed() {
						t.Logf("testing failed, test DB preserved at %s", path)
						return nil
					}
					if tsKeepFS {
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
	ts := &testStorage{
		t:              t,
		Storage:        stor,
		closeFn:        closeFn,
		opens:          make(map[uint64]bool),
		emuErrOnceMap:  make(map[uint64]uint),
		emuRandErrProb: 0x999,
		emuRandRand:    rand.New(rand.NewSource(0xfacedead)),
	}
	ts.cond.L = &ts.mu
	return ts
}
