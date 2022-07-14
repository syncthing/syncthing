// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// see readShortAt()
const randomBlockShift = 14 // 128k

// fakeFS is a fake filesystem for testing and benchmarking. It has the
// following properties:
//
//   - File metadata is kept in RAM. Specifically, we remember which files and
//     directories exist, their dates, permissions and sizes. Symlinks are
//     not supported.
//
//   - File contents are generated pseudorandomly with just the file name as
//     seed. Writes are discarded, other than having the effect of increasing
//     the file size. If you only write data that you've read from a file with
//     the same name on a different fakeFS, you'll never know the difference...
//
// - We totally ignore permissions - pretend you are root.
//
//   - The root path can contain URL query-style parameters that pre populate
//     the filesystem at creation with a certain amount of random data:
//
//     files=n    to generate n random files (default 0)
//     maxsize=n  to generate files up to a total of n MiB (default 0)
//     sizeavg=n  to set the average size of random files, in bytes (default 1<<20)
//     seed=n     to set the initial random seed (default 0)
//     insens=b   "true" makes filesystem case-insensitive Windows- or OSX-style (default false)
//     latency=d  to set the amount of time each "disk" operation takes, where d is time.ParseDuration format
//
// - Two fakeFS:s pointing at the same root path see the same files.
type fakeFS struct {
	counters    fakeFSCounters
	uri         string
	mut         sync.Mutex
	root        *fakeEntry
	insens      bool
	withContent bool
	latency     time.Duration
	PlatformDataGetter
}

type fakeFSCounters struct {
	Chmod       int64
	Lchown      int64
	Chtimes     int64
	Create      int64
	DirNames    int64
	Lstat       int64
	Mkdir       int64
	MkdirAll    int64
	Open        int64
	OpenFile    int64
	ReadSymlink int64
	Remove      int64
	RemoveAll   int64
	Rename      int64
}

var (
	fakeFSMut   sync.Mutex
	fakeFSCache = make(map[string]*fakeFS)
)

func newFakeFilesystem(rootURI string, _ ...Option) *fakeFS {
	fakeFSMut.Lock()
	defer fakeFSMut.Unlock()

	var params url.Values
	uri, err := url.Parse(rootURI)
	if err == nil {
		params = uri.Query()
	}

	if fs, ok := fakeFSCache[rootURI]; ok {
		// Already have an fs at this path
		return fs
	}

	fs := &fakeFS{
		uri: "fake://" + rootURI,
		root: &fakeEntry{
			name:      "/",
			entryType: fakeEntryTypeDir,
			mode:      0700,
			mtime:     time.Now(),
			children:  make(map[string]*fakeEntry),
		},
	}
	fs.PlatformDataGetter = NewUnixDataGetter(fs)

	files, _ := strconv.Atoi(params.Get("files"))
	maxsize, _ := strconv.Atoi(params.Get("maxsize"))
	sizeavg, _ := strconv.Atoi(params.Get("sizeavg"))
	seed, _ := strconv.Atoi(params.Get("seed"))

	fs.insens = params.Get("insens") == "true"
	fs.withContent = params.Get("content") == "true"

	if sizeavg == 0 {
		sizeavg = 1 << 20
	}

	if files > 0 || maxsize > 0 {
		// Generate initial data according to specs. Operations in here
		// *look* like file I/O, but they are not. Do not worry that they
		// might fail.

		rng := rand.New(rand.NewSource(int64(seed)))
		var createdFiles int
		var writtenData int64
		for (files == 0 || createdFiles < files) && (maxsize == 0 || writtenData>>20 < int64(maxsize)) {
			dir := filepath.Join(fmt.Sprintf("%02x", rng.Intn(255)), fmt.Sprintf("%02x", rng.Intn(255)))
			file := fmt.Sprintf("%016x", rng.Int63())
			fs.MkdirAll(dir, 0755)

			fd, _ := fs.Create(filepath.Join(dir, file))
			createdFiles++

			fsize := int64(sizeavg/2 + rng.Intn(sizeavg))
			fd.Truncate(fsize)
			writtenData += fsize

			ftime := time.Unix(1000000000+rng.Int63n(10*365*86400), 0)
			fs.Chtimes(filepath.Join(dir, file), ftime, ftime)
		}
	}

	// Also create a default folder marker for good measure
	fs.Mkdir(".stfolder", 0700)

	// We only set the latency after doing the operations required to create
	// the filesystem initially.
	fs.latency, _ = time.ParseDuration(params.Get("latency"))

	fakeFSCache[rootURI] = fs
	return fs
}

type fakeEntryType int

const (
	fakeEntryTypeFile fakeEntryType = iota
	fakeEntryTypeDir
	fakeEntryTypeSymlink
)

// fakeEntry is an entry (file or directory) in the fake filesystem
type fakeEntry struct {
	name      string
	entryType fakeEntryType
	dest      string // for symlinks
	size      int64
	mode      FileMode
	uid       int
	gid       int
	mtime     time.Time
	children  map[string]*fakeEntry
	content   []byte
}

func (fs *fakeFS) entryForName(name string) *fakeEntry {
	// bug: lookup doesn't work through symlinks.
	if fs.insens {
		name = UnicodeLowercaseNormalized(name)
	}

	name = filepath.ToSlash(name)
	if name == "." || name == "/" {
		return fs.root
	}

	name = strings.Trim(name, "/")
	comps := strings.Split(name, "/")
	entry := fs.root
	for _, comp := range comps {
		if entry.entryType != fakeEntryTypeDir {
			return nil
		}
		var ok bool
		entry, ok = entry.children[comp]
		if !ok {
			return nil
		}
	}
	return entry
}

func (fs *fakeFS) Chmod(name string, mode FileMode) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Chmod++
	time.Sleep(fs.latency)
	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	entry.mode = mode
	return nil
}

func (fs *fakeFS) Lchown(name, uid, gid string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Lchown++
	time.Sleep(fs.latency)
	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	entry.uid, _ = strconv.Atoi(uid)
	entry.gid, _ = strconv.Atoi(gid)
	return nil
}

func (fs *fakeFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Chtimes++
	time.Sleep(fs.latency)
	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	entry.mtime = mtime
	return nil
}

func (fs *fakeFS) create(name string) (*fakeEntry, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Create++
	time.Sleep(fs.latency)

	if entry := fs.entryForName(name); entry != nil {
		if entry.entryType == fakeEntryTypeDir {
			return nil, os.ErrExist
		} else if entry.entryType == fakeEntryTypeSymlink {
			return nil, errors.New("following symlink not supported")
		}
		entry.size = 0
		entry.mtime = time.Now()
		entry.mode = 0666
		entry.content = nil
		if fs.withContent {
			entry.content = make([]byte, 0)
		}
		return entry, nil
	}

	dir := filepath.Dir(name)
	base := filepath.Base(name)
	entry := fs.entryForName(dir)
	if entry == nil {
		return nil, os.ErrNotExist
	}
	new := &fakeEntry{
		name:  base,
		mode:  0666,
		mtime: time.Now(),
	}

	if fs.insens {
		base = UnicodeLowercaseNormalized(base)
	}

	if fs.withContent {
		new.content = make([]byte, 0)
	}

	entry.children[base] = new
	return new, nil
}

func (fs *fakeFS) Create(name string) (File, error) {
	entry, err := fs.create(name)
	if err != nil {
		return nil, err
	}
	if fs.insens {
		return &fakeFile{fakeEntry: entry, presentedName: filepath.Base(name)}, nil
	}
	return &fakeFile{fakeEntry: entry}, nil
}

func (fs *fakeFS) CreateSymlink(target, name string) error {
	entry, err := fs.create(name)
	if err != nil {
		return err
	}
	entry.entryType = fakeEntryTypeSymlink
	entry.dest = target
	return nil
}

func (fs *fakeFS) DirNames(name string) ([]string, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.DirNames++
	time.Sleep(fs.latency)

	entry := fs.entryForName(name)
	if entry == nil {
		return nil, os.ErrNotExist
	}

	names := make([]string, 0, len(entry.children))
	for _, child := range entry.children {
		names = append(names, child.name)
	}

	return names, nil
}

func (fs *fakeFS) Lstat(name string) (FileInfo, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Lstat++
	time.Sleep(fs.latency)

	entry := fs.entryForName(name)
	if entry == nil {
		return nil, os.ErrNotExist
	}

	info := &fakeFileInfo{*entry}
	if fs.insens {
		info.name = filepath.Base(name)
	}

	return info, nil
}

func (fs *fakeFS) Mkdir(name string, perm FileMode) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Mkdir++
	time.Sleep(fs.latency)

	dir := filepath.Dir(name)
	base := filepath.Base(name)
	entry := fs.entryForName(dir)
	key := base

	if entry == nil {
		return os.ErrNotExist
	}
	if entry.entryType != fakeEntryTypeDir {
		return os.ErrExist
	}
	if fs.insens {
		key = UnicodeLowercaseNormalized(key)
	}
	if _, ok := entry.children[key]; ok {
		return os.ErrExist
	}

	entry.children[key] = &fakeEntry{
		name:      base,
		entryType: fakeEntryTypeDir,
		mode:      perm,
		mtime:     time.Now(),
		children:  make(map[string]*fakeEntry),
	}
	return nil
}

func (fs *fakeFS) MkdirAll(name string, perm FileMode) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.MkdirAll++
	time.Sleep(fs.latency)

	name = filepath.ToSlash(name)
	name = strings.Trim(name, "/")
	comps := strings.Split(name, "/")
	entry := fs.root
	for _, comp := range comps {
		key := comp
		if fs.insens {
			key = UnicodeLowercaseNormalized(key)
		}

		next, ok := entry.children[key]

		if !ok {
			new := &fakeEntry{
				name:      comp,
				entryType: fakeEntryTypeDir,
				mode:      perm,
				mtime:     time.Now(),
				children:  make(map[string]*fakeEntry),
			}
			entry.children[key] = new
			next = new
		} else if next.entryType != fakeEntryTypeDir {
			return errors.New("not a directory")
		}

		entry = next
	}
	return nil
}

func (fs *fakeFS) Open(name string) (File, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Open++
	time.Sleep(fs.latency)

	entry := fs.entryForName(name)
	if entry == nil || entry.entryType != fakeEntryTypeFile {
		return nil, os.ErrNotExist
	}

	if fs.insens {
		return &fakeFile{fakeEntry: entry, presentedName: filepath.Base(name)}, nil
	}
	return &fakeFile{fakeEntry: entry}, nil
}

func (fs *fakeFS) OpenFile(name string, flags int, mode FileMode) (File, error) {
	if flags&os.O_CREATE == 0 {
		return fs.Open(name)
	}

	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.OpenFile++
	time.Sleep(fs.latency)

	dir := filepath.Dir(name)
	base := filepath.Base(name)
	entry := fs.entryForName(dir)
	key := base

	if entry == nil {
		return nil, os.ErrNotExist
	} else if entry.entryType != fakeEntryTypeDir {
		return nil, errors.New("not a directory")
	}

	if fs.insens {
		key = UnicodeLowercaseNormalized(key)
	}
	if flags&os.O_EXCL != 0 {
		if _, ok := entry.children[key]; ok {
			return nil, os.ErrExist
		}
	}

	newEntry := &fakeEntry{
		name:  base,
		mode:  mode,
		mtime: time.Now(),
	}
	if fs.withContent {
		newEntry.content = make([]byte, 0)
	}

	entry.children[key] = newEntry
	return &fakeFile{fakeEntry: newEntry}, nil
}

func (fs *fakeFS) ReadSymlink(name string) (string, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.ReadSymlink++
	time.Sleep(fs.latency)

	entry := fs.entryForName(name)
	if entry == nil {
		return "", os.ErrNotExist
	} else if entry.entryType != fakeEntryTypeSymlink {
		return "", errors.New("not a symlink")
	}
	return entry.dest, nil
}

func (fs *fakeFS) Remove(name string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Remove++
	time.Sleep(fs.latency)

	if fs.insens {
		name = UnicodeLowercaseNormalized(name)
	}

	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	if len(entry.children) != 0 {
		return errors.New("not empty")
	}

	entry = fs.entryForName(filepath.Dir(name))
	delete(entry.children, filepath.Base(name))
	return nil
}

func (fs *fakeFS) RemoveAll(name string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.RemoveAll++
	time.Sleep(fs.latency)

	if fs.insens {
		name = UnicodeLowercaseNormalized(name)
	}

	entry := fs.entryForName(filepath.Dir(name))
	if entry == nil {
		return nil // all tested real systems exhibit this behaviour
	}

	// RemoveAll is easy when the file system uses garbage collection under
	// the hood... We even get the correct semantics for open fd:s for free.
	delete(entry.children, filepath.Base(name))
	return nil
}

func (fs *fakeFS) Rename(oldname, newname string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	fs.counters.Rename++
	time.Sleep(fs.latency)

	oldKey := filepath.Base(oldname)
	newKey := filepath.Base(newname)

	if fs.insens {
		oldKey = UnicodeLowercaseNormalized(oldKey)
		newKey = UnicodeLowercaseNormalized(newKey)
	}

	p0 := fs.entryForName(filepath.Dir(oldname))
	if p0 == nil {
		return os.ErrNotExist
	}

	entry := p0.children[oldKey]
	if entry == nil {
		return os.ErrNotExist
	}

	p1 := fs.entryForName(filepath.Dir(newname))
	if p1 == nil {
		return os.ErrNotExist
	}

	dst, ok := p1.children[newKey]
	if ok {
		if fs.insens && newKey == oldKey {
			// case-only in-place rename
			entry.name = filepath.Base(newname)
			return nil
		}

		if dst.entryType == fakeEntryTypeDir {
			return errors.New("is a directory")
		}
	}

	p1.children[newKey] = entry
	entry.name = filepath.Base(newname)

	delete(p0.children, oldKey)

	return nil
}

func (fs *fakeFS) Stat(name string) (FileInfo, error) {
	return fs.Lstat(name)
}

func (fs *fakeFS) SymlinksSupported() bool {
	return false
}

func (fs *fakeFS) Walk(name string, walkFn WalkFunc) error {
	return errors.New("not implemented")
}

func (fs *fakeFS) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	return nil, nil, ErrWatchNotSupported
}

func (fs *fakeFS) Hide(name string) error {
	return nil
}

func (fs *fakeFS) Unhide(name string) error {
	return nil
}

func (fs *fakeFS) Glob(pattern string) ([]string, error) {
	// gnnh we don't seem to actually require this in practice
	return nil, errors.New("not implemented")
}

func (fs *fakeFS) Roots() ([]string, error) {
	return []string{"/"}, nil
}

func (fs *fakeFS) Usage(name string) (Usage, error) {
	return Usage{}, errors.New("not implemented")
}

func (fs *fakeFS) Type() FilesystemType {
	return FilesystemTypeFake
}

func (fs *fakeFS) URI() string {
	return fs.uri
}

func (fs *fakeFS) Options() []Option {
	return nil
}

func (fs *fakeFS) SameFile(fi1, fi2 FileInfo) bool {
	// BUG: real systems base file sameness on path, inodes, etc
	// we try our best, but FileInfo just doesn't have enough data
	// so there be false positives, especially on Windows
	// where ModTime is not that precise
	var ok bool
	if fs.insens {
		ok = UnicodeLowercaseNormalized(fi1.Name()) == UnicodeLowercaseNormalized(fi2.Name())
	} else {
		ok = fi1.Name() == fi2.Name()
	}

	return ok && fi1.ModTime().Equal(fi2.ModTime()) && fi1.Mode() == fi2.Mode() && fi1.IsDir() == fi2.IsDir() && fi1.IsRegular() == fi2.IsRegular() && fi1.IsSymlink() == fi2.IsSymlink() && fi1.Owner() == fi2.Owner() && fi1.Group() == fi2.Group()
}

func (fs *fakeFS) underlying() (Filesystem, bool) {
	return nil, false
}

func (fs *fakeFS) wrapperType() filesystemWrapperType {
	return filesystemWrapperTypeNone
}

func (fs *fakeFS) resetCounters() {
	fs.mut.Lock()
	fs.counters = fakeFSCounters{}
	fs.mut.Unlock()
}

func (fs *fakeFS) reportMetricsPerOp(b *testing.B) {
	fs.reportMetricsPer(b, 1, "op")
}

func (fs *fakeFS) reportMetricsPer(b *testing.B, divisor float64, unit string) {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	b.ReportMetric(float64(fs.counters.Lstat)/divisor/float64(b.N), "Lstat/"+unit)
	b.ReportMetric(float64(fs.counters.DirNames)/divisor/float64(b.N), "DirNames/"+unit)
}

// fakeFile is the representation of an open file. We don't care if it's
// opened for reading or writing, it's all good.
type fakeFile struct {
	*fakeEntry
	mut           sync.Mutex
	rng           io.Reader
	seed          int64
	offset        int64
	seedOffs      int64
	presentedName string // present (i.e. != "") on insensitive fs only
}

func (f *fakeFile) Close() error {
	return nil
}

func (f *fakeFile) Read(p []byte) (int, error) {
	f.mut.Lock()
	defer f.mut.Unlock()
	return f.readShortAt(p, f.offset)
}

func (f *fakeFile) ReadAt(p []byte, offs int64) (int, error) {
	f.mut.Lock()
	defer f.mut.Unlock()

	// ReadAt is spec:ed to always read a full block unless EOF or failure,
	// so we must loop. It's also not supposed to affect the seek position,
	// but that would make things annoying or inefficient in terms of
	// generating the appropriate RNG etc so I ignore that. In practice we
	// currently don't depend on that aspect of it...

	var read int
	for {
		n, err := f.readShortAt(p[read:], offs+int64(read))
		read += n
		if err != nil {
			return read, err
		}
		if read == len(p) {
			return read, nil
		}
	}
}

func (f *fakeFile) readShortAt(p []byte, offs int64) (int, error) {
	// Here be a certain amount of magic... We want to return pseudorandom,
	// predictable data so that a read from the same offset in the same file
	// always returns the same data. But the RNG is a stream, and reads can
	// be random.
	//
	// We split the file into "blocks" numbered by "seedNo", where each
	// block becomes an instantiation of the RNG, seeded with the hash of
	// the file number plus the seedNo (block number). We keep the RNG
	// around in the hope that the next read will be sequential to this one
	// and we can continue reading from the same RNG.
	//
	// When that's not the case we create a new RNG for the block we are in,
	// read as many bytes from it as necessary to get to the right offset,
	// and then serve the read from there. We limit the length of the read
	// to the end of the block, as another RNG needs to be created to serve
	// the next block.
	//
	// The size of the blocks are a matter of taste... Larger blocks give
	// better performance for sequential reads, but worse for random reads
	// as we often need to generate and throw away a lot of data at the
	// start of the block to serve a given read. 128 KiB blocks fit
	// reasonably well with the type of IO Syncthing tends to do.

	if f.entryType == fakeEntryTypeDir {
		return 0, errors.New("is a directory")
	}

	if offs >= f.size {
		return 0, io.EOF
	}

	if f.content != nil {
		n := copy(p, f.content[int(offs):])
		f.offset = offs + int64(n)
		return n, nil
	}

	// Lazily calculate our main seed, a simple 64 bit FNV hash our file
	// name.
	if f.seed == 0 {
		hf := fnv.New64()
		hf.Write([]byte(f.name))
		f.seed = int64(hf.Sum64())
	}

	// Check whether the read is a continuation of an RNG we already have or
	// we need to set up a new one.
	seedNo := offs >> randomBlockShift
	minOffs := seedNo << randomBlockShift
	nextBlockOffs := (seedNo + 1) << randomBlockShift
	if f.rng == nil || f.offset != offs || seedNo != f.seedOffs {
		// This is not a straight read continuing from a previous one
		f.rng = rand.New(rand.NewSource(f.seed + seedNo))

		// If the read is not at the start of the block, discard data
		// accordingly.
		diff := offs - minOffs
		if diff > 0 {
			lr := io.LimitReader(f.rng, diff)
			io.Copy(io.Discard, lr)
		}

		f.offset = offs
		f.seedOffs = seedNo
	}

	size := len(p)

	// Don't read past the end of the file
	if offs+int64(size) > f.size {
		size = int(f.size - offs)
	}

	// Don't read across the block boundary
	if offs+int64(size) > nextBlockOffs {
		size = int(nextBlockOffs - offs)
	}

	f.offset += int64(size)
	return f.rng.Read(p[:size])
}

func (f *fakeFile) Seek(offset int64, whence int) (int64, error) {
	f.mut.Lock()
	defer f.mut.Unlock()

	if f.entryType == fakeEntryTypeDir {
		return 0, errors.New("is a directory")
	}

	f.rng = nil

	switch whence {
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		f.offset = f.size - offset
	case io.SeekStart:
		f.offset = offset
	}
	if f.offset < 0 {
		f.offset = 0
		return f.offset, errors.New("seek before start")
	}
	if f.offset > f.size {
		f.offset = f.size
		return f.offset, io.EOF
	}
	return f.offset, nil
}

func (f *fakeFile) Write(p []byte) (int, error) {
	f.mut.Lock()
	offs := f.offset
	f.mut.Unlock()
	return f.WriteAt(p, offs)
}

func (f *fakeFile) WriteAt(p []byte, off int64) (int, error) {
	f.mut.Lock()
	defer f.mut.Unlock()

	if f.entryType == fakeEntryTypeDir {
		return 0, errors.New("is a directory")
	}

	if f.content != nil {
		if len(f.content) < int(off)+len(p) {
			newc := make([]byte, int(off)+len(p))
			copy(newc, f.content)
			f.content = newc
		}
		copy(f.content[int(off):], p)
	}

	f.rng = nil
	f.offset = off + int64(len(p))
	if f.offset > f.size {
		f.size = f.offset
	}
	return len(p), nil
}

func (f *fakeFile) Name() string {
	if f.presentedName != "" {
		return f.presentedName
	}
	return f.name
}

func (f *fakeFile) Truncate(size int64) error {
	f.mut.Lock()
	defer f.mut.Unlock()

	if f.content != nil {
		if int64(cap(f.content)) < size {
			c := make([]byte, size)
			copy(c[:len(f.content)], f.content)
			f.content = c
		} else {
			f.content = f.content[:int(size)]
		}
	}
	f.rng = nil
	f.size = size
	if f.offset > size {
		f.offset = size
	}
	return nil
}

func (f *fakeFile) Stat() (FileInfo, error) {
	info := &fakeFileInfo{*f.fakeEntry}
	if f.presentedName != "" {
		info.name = f.presentedName
	}

	return info, nil
}

func (f *fakeFile) Sync() error {
	return nil
}

// fakeFileInfo is the stat result.
type fakeFileInfo struct {
	fakeEntry // intentionally a copy of the struct
}

func (f *fakeFileInfo) Name() string {
	return f.name
}

func (f *fakeFileInfo) Mode() FileMode {
	return f.mode
}

func (f *fakeFileInfo) Size() int64 {
	return f.size
}

func (f *fakeFileInfo) ModTime() time.Time {
	return f.mtime
}

func (f *fakeFileInfo) IsDir() bool {
	return f.entryType == fakeEntryTypeDir
}

func (f *fakeFileInfo) IsRegular() bool {
	return f.entryType == fakeEntryTypeFile
}

func (f *fakeFileInfo) IsSymlink() bool {
	return f.entryType == fakeEntryTypeSymlink
}

func (f *fakeFileInfo) Owner() int {
	return f.uid
}

func (f *fakeFileInfo) Group() int {
	return f.gid
}
