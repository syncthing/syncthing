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
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// see readShortAt()
const randomBlockShift = 14 // 128k

// fakefs is a fake filesystem for testing and benchmarking. It has the
// following properties:
//
// - File metadata is kept in RAM. Specifically, we remember which files and
//   directories exist, their dates, permissions and sizes. Symlinks are
//   not supported.
//
// - File contents are generated pseudorandomly with just the file name as
//   seed. Writes are discarded, other than having the effect of increasing
//   the file size. If you only write data that you've read from a file with
//   the same name on a different fakefs, you'll never know the difference...
//
// - We totally ignore permissions - pretend you are root.
//
// - The root path can contain URL query-style parameters that pre populate
//   the filesystem at creation with a certain amount of random data:
//
//     files=n    to generate n random files (default 0)
//     maxsize=n  to generate files up to a total of n MiB (default 0)
//     sizeavg=n  to set the average size of random files, in bytes (default 1<<20)
//     seed=n     to set the initial random seed (default 0)
//
// - Two fakefs:s pointing at the same root path see the same files.
//
type fakefs struct {
	mut  sync.Mutex
	root *fakeEntry
}

var (
	fakefsMut sync.Mutex
	fakefsFs  = make(map[string]*fakefs)
)

func newFakeFilesystem(root string) *fakefs {
	fakefsMut.Lock()
	defer fakefsMut.Unlock()

	var params url.Values
	uri, err := url.Parse(root)
	if err == nil {
		root = uri.Path
		params = uri.Query()
	}

	if fs, ok := fakefsFs[root]; ok {
		// Already have an fs at this path
		return fs
	}

	fs := &fakefs{
		root: &fakeEntry{
			name:      "/",
			entryType: fakeEntryTypeDir,
			mode:      0700,
			mtime:     time.Now(),
			children:  make(map[string]*fakeEntry),
		},
	}

	files, _ := strconv.Atoi(params.Get("files"))
	maxsize, _ := strconv.Atoi(params.Get("maxsize"))
	sizeavg, _ := strconv.Atoi(params.Get("sizeavg"))
	seed, _ := strconv.Atoi(params.Get("seed"))
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

	fakefsFs[root] = fs
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
}

func (fs *fakefs) entryForName(name string) *fakeEntry {
	// bug: lookup doesn't work through symlinks.

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

func (fs *fakefs) Chmod(name string, mode FileMode) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	entry.mode = mode
	return nil
}

func (fs *fakefs) Lchown(name string, uid, gid int) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	entry.uid = uid
	entry.gid = gid
	return nil
}

func (fs *fakefs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()
	entry := fs.entryForName(name)
	if entry == nil {
		return os.ErrNotExist
	}
	entry.mtime = mtime
	return nil
}

func (fs *fakefs) create(name string) (*fakeEntry, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	if entry := fs.entryForName(name); entry != nil {
		if entry.entryType == fakeEntryTypeDir {
			return nil, os.ErrExist
		} else if entry.entryType == fakeEntryTypeSymlink {
			return nil, errors.New("following symlink not supported")
		}
		entry.size = 0
		entry.mtime = time.Now()
		entry.mode = 0666
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
	entry.children[base] = new
	return new, nil
}

func (fs *fakefs) Create(name string) (File, error) {
	entry, err := fs.create(name)
	if err != nil {
		return nil, err
	}
	return &fakeFile{fakeEntry: entry}, nil
}

func (fs *fakefs) CreateSymlink(target, name string) error {
	entry, err := fs.create(name)
	if err != nil {
		return err
	}
	entry.entryType = fakeEntryTypeSymlink
	entry.dest = target
	return nil
}

func (fs *fakefs) DirNames(name string) ([]string, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	entry := fs.entryForName(name)
	if entry == nil {
		return nil, os.ErrNotExist
	}

	names := make([]string, 0, len(entry.children))
	for name := range entry.children {
		names = append(names, name)
	}

	return names, nil
}

func (fs *fakefs) Lstat(name string) (FileInfo, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	entry := fs.entryForName(name)
	if entry == nil {
		return nil, os.ErrNotExist
	}
	return &fakeFileInfo{*entry}, nil
}

func (fs *fakefs) Mkdir(name string, perm FileMode) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	dir := filepath.Dir(name)
	base := filepath.Base(name)
	entry := fs.entryForName(dir)
	if entry == nil {
		return os.ErrNotExist
	}
	if entry.entryType != fakeEntryTypeDir {
		return os.ErrExist
	}
	if _, ok := entry.children[base]; ok {
		return os.ErrExist
	}

	entry.children[base] = &fakeEntry{
		name:      base,
		entryType: fakeEntryTypeDir,
		mode:      perm,
		mtime:     time.Now(),
		children:  make(map[string]*fakeEntry),
	}
	return nil
}

func (fs *fakefs) MkdirAll(name string, perm FileMode) error {
	name = filepath.ToSlash(name)
	name = strings.Trim(name, "/")
	comps := strings.Split(name, "/")
	entry := fs.root
	for _, comp := range comps {
		next, ok := entry.children[comp]

		if !ok {
			new := &fakeEntry{
				name:      comp,
				entryType: fakeEntryTypeDir,
				mode:      perm,
				mtime:     time.Now(),
				children:  make(map[string]*fakeEntry),
			}
			entry.children[comp] = new
			next = new
		} else if next.entryType != fakeEntryTypeDir {
			return errors.New("not a directory")
		}

		entry = next
	}
	return nil
}

func (fs *fakefs) Open(name string) (File, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	entry := fs.entryForName(name)
	if entry == nil || entry.entryType != fakeEntryTypeFile {
		return nil, os.ErrNotExist
	}
	return &fakeFile{fakeEntry: entry}, nil
}

func (fs *fakefs) OpenFile(name string, flags int, mode FileMode) (File, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	if flags&os.O_CREATE == 0 {
		return fs.Open(name)
	}

	dir := filepath.Dir(name)
	base := filepath.Base(name)
	entry := fs.entryForName(dir)
	if entry == nil {
		return nil, os.ErrNotExist
	} else if entry.entryType != fakeEntryTypeDir {
		return nil, errors.New("not a directory")
	}

	if flags&os.O_EXCL != 0 {
		if _, ok := entry.children[base]; ok {
			return nil, os.ErrExist
		}
	}

	newEntry := &fakeEntry{
		name:  base,
		mode:  mode,
		mtime: time.Now(),
	}

	entry.children[base] = newEntry
	return &fakeFile{fakeEntry: newEntry}, nil
}

func (fs *fakefs) ReadSymlink(name string) (string, error) {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	entry := fs.entryForName(name)
	if entry == nil {
		return "", os.ErrNotExist
	} else if entry.entryType != fakeEntryTypeSymlink {
		return "", errors.New("not a symlink")
	}
	return entry.dest, nil
}

func (fs *fakefs) Remove(name string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()

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

func (fs *fakefs) RemoveAll(name string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	entry := fs.entryForName(filepath.Dir(name))
	if entry == nil {
		return os.ErrNotExist
	}

	// RemoveAll is easy when the file system uses garbage collection under
	// the hood... We even get the correct semantics for open fd:s for free.
	delete(entry.children, filepath.Base(name))
	return nil
}

func (fs *fakefs) Rename(oldname, newname string) error {
	fs.mut.Lock()
	defer fs.mut.Unlock()

	p0 := fs.entryForName(filepath.Dir(oldname))
	if p0 == nil {
		return os.ErrNotExist
	}

	entry := p0.children[filepath.Base(oldname)]
	if entry == nil {
		return os.ErrNotExist
	}

	p1 := fs.entryForName(filepath.Dir(newname))
	if p1 == nil {
		return os.ErrNotExist
	}

	dst, ok := p1.children[filepath.Base(newname)]
	if ok && dst.entryType == fakeEntryTypeDir {
		return errors.New("is a directory")
	}

	p1.children[filepath.Base(newname)] = entry
	delete(p0.children, filepath.Base(oldname))
	return nil
}

func (fs *fakefs) Stat(name string) (FileInfo, error) {
	return fs.Lstat(name)
}

func (fs *fakefs) SymlinksSupported() bool {
	return false
}

func (fs *fakefs) Walk(name string, walkFn WalkFunc) error {
	return errors.New("not implemented")
}

func (fs *fakefs) Watch(path string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	return nil, nil, ErrWatchNotSupported
}

func (fs *fakefs) Hide(name string) error {
	return nil
}

func (fs *fakefs) Unhide(name string) error {
	return nil
}

func (fs *fakefs) Glob(pattern string) ([]string, error) {
	// gnnh we don't seem to actually require this in practice
	return nil, errors.New("not implemented")
}

func (fs *fakefs) Roots() ([]string, error) {
	return []string{"/"}, nil
}

func (fs *fakefs) Usage(name string) (Usage, error) {
	return Usage{}, errors.New("not implemented")
}

func (fs *fakefs) Type() FilesystemType {
	return FilesystemTypeFake
}

func (fs *fakefs) URI() string {
	return "fake://" + fs.root.name
}

func (fs *fakefs) SameFile(fi1, fi2 FileInfo) bool {
	return fi1.Name() == fi2.Name()
}

// fakeFile is the representation of an open file. We don't care if it's
// opened for reading or writing, it's all good.
type fakeFile struct {
	*fakeEntry
	mut      sync.Mutex
	rng      io.Reader
	seed     int64
	offset   int64
	seedOffs int64
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
			io.Copy(ioutil.Discard, lr)
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
	return f.WriteAt(p, f.offset)
}

func (f *fakeFile) WriteAt(p []byte, off int64) (int, error) {
	f.mut.Lock()
	defer f.mut.Unlock()

	if f.entryType == fakeEntryTypeDir {
		return 0, errors.New("is a directory")
	}

	f.rng = nil
	f.offset = off + int64(len(p))
	if f.offset > f.size {
		f.size = f.offset
	}
	return len(p), nil
}

func (f *fakeFile) Name() string {
	return f.name
}

func (f *fakeFile) Truncate(size int64) error {
	f.mut.Lock()
	defer f.mut.Unlock()

	f.rng = nil
	f.size = size
	if f.offset > size {
		f.offset = size
	}
	return nil
}

func (f *fakeFile) Stat() (FileInfo, error) {
	return &fakeFileInfo{*f.fakeEntry}, nil
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
