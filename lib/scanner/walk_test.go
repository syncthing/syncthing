// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	rdebug "runtime/debug"
	"sort"
	"sync"
	"testing"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
	"golang.org/x/text/unicode/norm"
)

type testfile struct {
	name   string
	length int64
	hash   string
}

type testfileList []testfile

var testdata = testfileList{
	{"afile", 4, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"},
	{"dir1", 128, ""},
	{filepath.Join("dir1", "dfile"), 5, "49ae93732fcf8d63fe1cce759664982dbd5b23161f007dba8561862adc96d063"},
	{"dir2", 128, ""},
	{filepath.Join("dir2", "cfile"), 4, "bf07a7fbb825fc0aae7bf4a1177b2b31fcf8a3feeaf7092761e18c859ee52a9c"},
	{"excludes", 37, "df90b52f0c55dba7a7a940affe482571563b1ac57bd5be4d8a0291e7de928e06"},
	{"further-excludes", 5, "7eb0a548094fa6295f7fd9200d69973e5f5ec5c04f2a86d998080ac43ecf89f1"},
}

func init() {
	// This test runs the risk of entering infinite recursion if it fails.
	// Limit the stack size to 10 megs to crash early in that case instead of
	// potentially taking down the box...
	rdebug.SetMaxStack(10 * 1 << 20)
}

func newTestFs(opts ...fs.Option) fs.Filesystem {
	// This mirrors some test data we used to have in a physical `testdata`
	// directory here.
	tfs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(16)+"?content=true&nostfolder=true", opts...)
	tfs.Mkdir("dir1", 0o755)
	tfs.Mkdir("dir2", 0o755)
	tfs.Mkdir("dir3", 0o755)
	tfs.MkdirAll("dir2/dir21/dir22/dir23", 0o755)
	tfs.MkdirAll("dir2/dir21/dir22/efile", 0o755)
	tfs.MkdirAll("dir2/dir21/dira", 0o755)
	tfs.MkdirAll("dir2/dir21/efile/ign", 0o755)
	fs.WriteFile(tfs, "dir1/cfile", []byte("baz\n"), 0o644)
	fs.WriteFile(tfs, "dir1/dfile", []byte("quux\n"), 0o644)
	fs.WriteFile(tfs, "dir2/cfile", []byte("baz\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dfile", []byte("quux\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/dir22/dir23/efile", []byte("\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/dir22/efile/efile", []byte("\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/dir22/efile/ign/efile", []byte("\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/dira/efile", []byte("\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/dira/ffile", []byte("\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/efile/ign/efile", []byte("\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/cfile", []byte("foo\n"), 0o644)
	fs.WriteFile(tfs, "dir2/dir21/dfile", []byte("quux\n"), 0o644)
	fs.WriteFile(tfs, "dir3/cfile", []byte("foo\n"), 0o644)
	fs.WriteFile(tfs, "dir3/dfile", []byte("quux\n"), 0o644)
	fs.WriteFile(tfs, "afile", []byte("foo\n"), 0o644)
	fs.WriteFile(tfs, "bfile", []byte("bar\n"), 0o644)
	fs.WriteFile(tfs, ".stignore", []byte("#include excludes\n\nbfile\ndir1/cfile\n/dir2/dir21\n"), 0o644)
	fs.WriteFile(tfs, "excludes", []byte("dir2/dfile\n#include further-excludes\n"), 0o644)
	fs.WriteFile(tfs, "further-excludes", []byte("dir3\n"), 0o644)
	return tfs
}

func TestWalkSub(t *testing.T) {
	testFs := newTestFs()
	ignores := ignore.New(testFs)
	err := ignores.Load(".stignore")
	if err != nil {
		t.Fatal(err)
	}

	cfg, cancel := testConfig()
	defer cancel()
	cfg.Subs = []string{"dir2"}
	cfg.Matcher = ignores
	fchan := Walk(context.TODO(), cfg)
	var files []protocol.FileInfo
	for f := range fchan {
		if f.Err != nil {
			t.Errorf("Error while scanning %v: %v", f.Err, f.Path)
		}
		files = append(files, f.File)
	}

	// The directory contains two files, where one is ignored from a higher
	// level. We should see only the directory and one of the files.

	if len(files) != 2 {
		t.Fatalf("Incorrect length %d != 2", len(files))
	}
	if files[0].Name != "dir2" {
		t.Errorf("Incorrect file %v != dir2", files[0])
	}
	if files[1].Name != filepath.Join("dir2", "cfile") {
		t.Errorf("Incorrect file %v != dir2/cfile", files[1])
	}
}

func TestWalk(t *testing.T) {
	testFs := newTestFs()
	ignores := ignore.New(testFs)
	err := ignores.Load(".stignore")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ignores)

	cfg, cancel := testConfig()
	defer cancel()
	cfg.Matcher = ignores
	fchan := Walk(context.TODO(), cfg)

	var tmp []protocol.FileInfo
	for f := range fchan {
		if f.Err != nil {
			t.Errorf("Error while scanning %v: %v", f.Err, f.Path)
		}
		tmp = append(tmp, f.File)
	}
	sort.Sort(fileList(tmp))
	files := fileList(tmp).testfiles()

	if diff, equal := messagediff.PrettyDiff(testdata, files); !equal {
		t.Errorf("Walk returned unexpected data. Diff:\n%s", diff)
		t.Error(testdata[4], files[4])
	}
}

func TestVerify(t *testing.T) {
	blocksize := 16
	// data should be an even multiple of blocksize long
	data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut e")
	buf := bytes.NewBuffer(data)
	progress := newByteCounter()
	defer progress.Close()

	blocks, err := Blocks(context.TODO(), buf, blocksize, -1, progress, false)
	if err != nil {
		t.Fatal(err)
	}
	if exp := len(data) / blocksize; len(blocks) != exp {
		t.Fatalf("Incorrect number of blocks %d != %d", len(blocks), exp)
	}

	if int64(len(data)) != progress.Total() {
		t.Fatalf("Incorrect counter value %d  != %d", len(data), progress.Total())
	}

	buf = bytes.NewBuffer(data)
	err = verify(buf, blocksize, blocks)
	t.Log(err)
	if err != nil {
		t.Fatal("Unexpected verify failure", err)
	}

	buf = bytes.NewBuffer(append(data, '\n'))
	err = verify(buf, blocksize, blocks)
	t.Log(err)
	if err == nil {
		t.Fatal("Unexpected verify success")
	}

	buf = bytes.NewBuffer(data[:len(data)-1])
	err = verify(buf, blocksize, blocks)
	t.Log(err)
	if err == nil {
		t.Fatal("Unexpected verify success")
	}

	data[42] = 42
	buf = bytes.NewBuffer(data)
	err = verify(buf, blocksize, blocks)
	t.Log(err)
	if err == nil {
		t.Fatal("Unexpected verify success")
	}
}

func TestNormalization(t *testing.T) {
	if build.IsDarwin {
		t.Skip("Normalization test not possible on darwin")
		return
	}

	testFs := newTestFs()

	tests := []string{
		"0-A",            // ASCII A -- accepted
		"1-\xC3\x84",     // NFC 'Ä' -- conflicts with the entry below, accepted
		"1-\x41\xCC\x88", // NFD 'Ä' -- conflicts with the entry above, ignored
		"2-\xC3\x85",     // NFC 'Å' -- accepted
		"3-\x41\xCC\x83", // NFD 'Ã' -- converted to NFC
		"4-\xE2\x98\x95", // U+2615 HOT BEVERAGE (☕) -- accepted
		"5-\xCD\xE2",     // EUC-CN "wài" (外) -- ignored (not UTF8)
	}
	numInvalid := 2

	numValid := len(tests) - numInvalid

	for _, s1 := range tests {
		// Create a directory for each of the interesting strings above
		if err := testFs.MkdirAll(filepath.Join("normalization", s1), 0o755); err != nil {
			t.Fatal(err)
		}

		for _, s2 := range tests {
			// Within each dir, create a file with each of the interesting
			// file names. Ensure that the file doesn't exist when it's
			// created. This detects and fails if there's file name
			// normalization stuff at the filesystem level.
			if fd, err := testFs.OpenFile(filepath.Join("normalization", s1, s2), os.O_CREATE|os.O_EXCL, 0o644); err != nil {
				t.Fatal(err)
			} else {
				fd.Write([]byte("test"))
				fd.Close()
			}
		}
	}

	// We can normalize a directory name, but we can't descend into it in the
	// same pass due to how filepath.Walk works. So we run the scan twice to
	// make sure it all gets done. In production, things will be correct
	// eventually...

	walkDir(testFs, "normalization", nil, nil, 0)
	tmp := walkDir(testFs, "normalization", nil, nil, 0)

	files := fileList(tmp).testfiles()

	// We should have one file per combination, plus the directories
	// themselves, plus the "testdata/normalization" directory

	expectedNum := numValid*numValid + numValid + 1
	if len(files) != expectedNum {
		t.Errorf("Expected %d files, got %d, numvalid %d", expectedNum, len(files), numValid)
	}

	// The file names should all be in NFC form.

	for _, f := range files {
		t.Logf("%q (% x) %v", f.name, f.name, norm.NFC.IsNormalString(f.name))
		if !norm.NFC.IsNormalString(f.name) {
			t.Errorf("File name %q is not NFC normalized", f.name)
		}
	}
}

func TestNormalizationDarwinCaseFS(t *testing.T) {
	// This tests that normalization works on Darwin, through a CaseFS.

	if !build.IsDarwin {
		t.Skip("Normalization test not possible on non-Darwin")
		return
	}

	testFs := newTestFs(new(fs.OptionDetectCaseConflicts))

	testFs.RemoveAll("normalization")
	defer testFs.RemoveAll("normalization")
	testFs.MkdirAll("normalization", 0o755)

	const (
		inNFC = "\xC3\x84"
		inNFD = "\x41\xCC\x88"
	)

	// Create dir in NFC
	if err := testFs.Mkdir(filepath.Join("normalization", "dir-"+inNFC), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create file in NFC
	fd, err := testFs.Create(filepath.Join("normalization", "dir-"+inNFC, "file-"+inNFC))
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// Walk, which should normalize and return
	walkDir(testFs, "normalization", nil, nil, 0)
	tmp := walkDir(testFs, "normalization", nil, nil, 0)
	if len(tmp) != 3 {
		t.Error("Expected one file and one dir scanned")
	}

	// Verify we see the normalized entries in the result
	foundFile := false
	foundDir := false
	for _, f := range tmp {
		if f.Name == filepath.Join("normalization", "dir-"+inNFD) {
			foundDir = true
			continue
		}
		if f.Name == filepath.Join("normalization", "dir-"+inNFD, "file-"+inNFD) {
			foundFile = true
			continue
		}
	}
	if !foundFile || !foundDir {
		t.Error("Didn't find expected normalization form")
	}
}

func TestIssue1507(_ *testing.T) {
	w := &walker{}
	w.Matcher = ignore.New(w.Filesystem)
	h := make(chan protocol.FileInfo, 100)
	f := make(chan ScanResult, 100)
	fn := w.walkAndHashFiles(context.TODO(), h, f)

	fn("", nil, protocol.ErrClosed)
}

func TestWalkSymlinkUnix(t *testing.T) {
	if build.IsWindows {
		t.Skip("skipping unsupported symlink test")
		return
	}

	// Create a folder with a symlink in it
	os.RemoveAll("_symlinks")
	os.Mkdir("_symlinks", 0o755)
	defer os.RemoveAll("_symlinks")
	os.Symlink("../testdata", "_symlinks/link")

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, "_symlinks")
	for _, path := range []string{".", "link"} {
		// Scan it
		files := walkDir(fs, path, nil, nil, 0)

		// Verify that we got one symlink and with the correct attributes
		if len(files) != 1 {
			t.Errorf("expected 1 symlink, not %d", len(files))
		}
		if len(files[0].Blocks) != 0 {
			t.Errorf("expected zero blocks for symlink, not %d", len(files[0].Blocks))
		}
		if files[0].SymlinkTarget != "../testdata" {
			t.Errorf("expected symlink to have target destination, not %q", files[0].SymlinkTarget)
		}
	}
}

func TestBlocksizeHysteresis(t *testing.T) {
	// Verify that we select the right block size in the presence of old
	// file information.

	if testing.Short() {
		t.Skip("long and hard test")
	}

	sf := fs.NewWalkFilesystem(&singleFileFS{
		name:     "testfile.dat",
		filesize: 500 << 20, // 500 MiB
	})

	current := make(fakeCurrentFiler)

	runTest := func(expectedBlockSize int) {
		files := walkDir(sf, ".", current, nil, 0)
		if len(files) != 1 {
			t.Fatalf("expected one file, not %d", len(files))
		}
		if s := files[0].BlockSize(); s != expectedBlockSize {
			t.Fatalf("incorrect block size %d != expected %d", s, expectedBlockSize)
		}
	}

	// Scan with no previous knowledge. We should get a 512 KiB block size.

	runTest(512 << 10)

	// Scan on the assumption that previous size was 256 KiB. Retain 256 KiB
	// block size.

	current["testfile.dat"] = protocol.FileInfo{
		Name:         "testfile.dat",
		Size:         500 << 20,
		RawBlockSize: 256 << 10,
	}
	runTest(256 << 10)

	// Scan on the assumption that previous size was 1 MiB. Retain 1 MiB
	// block size.

	current["testfile.dat"] = protocol.FileInfo{
		Name:         "testfile.dat",
		Size:         500 << 20,
		RawBlockSize: 1 << 20,
	}
	runTest(1 << 20)

	// Scan on the assumption that previous size was 128 KiB. Move to 512
	// KiB because the difference is large.

	current["testfile.dat"] = protocol.FileInfo{
		Name:         "testfile.dat",
		Size:         500 << 20,
		RawBlockSize: 128 << 10,
	}
	runTest(512 << 10)

	// Scan on the assumption that previous size was 2 MiB. Move to 512
	// KiB because the difference is large.

	current["testfile.dat"] = protocol.FileInfo{
		Name:         "testfile.dat",
		Size:         500 << 20,
		RawBlockSize: 2 << 20,
	}
	runTest(512 << 10)
}

func TestWalkReceiveOnly(t *testing.T) {
	sf := fs.NewWalkFilesystem(&singleFileFS{
		name:     "testfile.dat",
		filesize: 1024,
	})

	current := make(fakeCurrentFiler)

	// Initial scan, no files in the CurrentFiler. Should pick up the file and
	// set the ReceiveOnly flag on it, because that's the flag we give the
	// walker to set.

	files := walkDir(sf, ".", current, nil, protocol.FlagLocalReceiveOnly)
	if len(files) != 1 {
		t.Fatal("Should have scanned one file")
	}

	if files[0].LocalFlags != protocol.FlagLocalReceiveOnly {
		t.Fatal("Should have set the ReceiveOnly flag")
	}

	// Update the CurrentFiler and scan again. It should not return
	// anything, because the file has not changed. This verifies that the
	// ReceiveOnly flag is properly ignored and doesn't trigger a rescan
	// every time.

	cur := files[0]
	current[cur.Name] = cur

	files = walkDir(sf, ".", current, nil, protocol.FlagLocalReceiveOnly)
	if len(files) != 0 {
		t.Fatal("Should not have scanned anything")
	}

	// Now pretend the file was previously ignored instead. We should pick up
	// the difference in flags and set just the LocalReceive flags.

	cur.LocalFlags = protocol.FlagLocalIgnored
	current[cur.Name] = cur

	files = walkDir(sf, ".", current, nil, protocol.FlagLocalReceiveOnly)
	if len(files) != 1 {
		t.Fatal("Should have scanned one file")
	}

	if files[0].LocalFlags != protocol.FlagLocalReceiveOnly {
		t.Fatal("Should have set the ReceiveOnly flag")
	}
}

func TestScanOwnershipPOSIX(t *testing.T) {
	// This test works on all operating systems because the FakeFS is always POSIXy.

	fakeFS := fs.NewFilesystem(fs.FilesystemTypeFake, "TestScanOwnership")
	current := make(fakeCurrentFiler)

	fakeFS.Create("root-owned")
	fakeFS.Create("user-owned")
	fakeFS.Lchown("user-owned", "1234", "5678")
	fakeFS.Mkdir("user-owned-dir", 0o755)
	fakeFS.Lchown("user-owned-dir", "2345", "6789")

	expected := []struct {
		name     string
		uid, gid int
	}{
		{"root-owned", 0, 0},
		{"user-owned", 1234, 5678},
		{"user-owned-dir", 2345, 6789},
	}

	files := walkDir(fakeFS, ".", current, nil, 0)
	if len(files) != len(expected) {
		t.Fatalf("expected %d items, not %d", len(expected), len(files))
	}
	for i := range expected {
		if files[i].Name != expected[i].name {
			t.Errorf("expected %s, got %s", expected[i].name, files[i].Name)
			continue
		}

		if files[i].Platform.Unix == nil {
			t.Error("failed to load POSIX data on", files[i].Name)
			continue
		}
		if files[i].Platform.Unix.UID != expected[i].uid {
			t.Errorf("expected %d, got %d", expected[i].uid, files[i].Platform.Unix.UID)
		}
		if files[i].Platform.Unix.GID != expected[i].gid {
			t.Errorf("expected %d, got %d", expected[i].gid, files[i].Platform.Unix.GID)
		}
	}
}

func TestScanOwnershipWindows(t *testing.T) {
	if !build.IsWindows {
		t.Skip("This test only works on Windows")
	}

	testFS := fs.NewFilesystem(fs.FilesystemTypeBasic, t.TempDir())
	current := make(fakeCurrentFiler)

	fd, err := testFS.Create("user-owned")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	files := walkDir(testFS, ".", current, nil, 0)
	if len(files) != 1 {
		t.Fatalf("expected %d items, not %d", 1, len(files))
	}
	t.Log(files[0])

	// The file should have an owner name set.
	if files[0].Platform.Windows == nil {
		t.Fatal("failed to load Windows data")
	}
	if files[0].Platform.Windows.OwnerName == "" {
		t.Errorf("expected owner name to be set")
	}
}

func walkDir(fs fs.Filesystem, dir string, cfiler CurrentFiler, matcher *ignore.Matcher, localFlags uint32) []protocol.FileInfo {
	cfg, cancel := testConfig()
	defer cancel()
	cfg.Filesystem = fs
	cfg.Subs = []string{dir}
	cfg.AutoNormalize = true
	cfg.CurrentFiler = cfiler
	cfg.Matcher = matcher
	cfg.LocalFlags = localFlags
	cfg.ScanOwnership = true
	fchan := Walk(context.TODO(), cfg)

	var tmp []protocol.FileInfo
	for f := range fchan {
		if f.Err == nil {
			tmp = append(tmp, f.File)
		}
	}
	sort.Sort(fileList(tmp))

	return tmp
}

type fileList []protocol.FileInfo

func (l fileList) Len() int {
	return len(l)
}

func (l fileList) Less(a, b int) bool {
	return l[a].Name < l[b].Name
}

func (l fileList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l fileList) testfiles() testfileList {
	testfiles := make(testfileList, len(l))
	for i, f := range l {
		if len(f.Blocks) > 1 {
			panic("simple test case stuff only supports a single block per file")
		}
		testfiles[i] = testfile{name: f.Name, length: f.FileSize()}
		if len(f.Blocks) == 1 {
			testfiles[i].hash = fmt.Sprintf("%x", f.Blocks[0].Hash)
		}
	}
	return testfiles
}

func (l testfileList) String() string {
	var b bytes.Buffer
	b.WriteString("{\n")
	for _, f := range l {
		fmt.Fprintf(&b, "  %s (%d bytes): %s\n", f.name, f.length, f.hash)
	}
	b.WriteString("}")
	return b.String()
}

var initOnce sync.Once

const (
	testdataSize = 17<<20 + 1
	testdataName = "_random.data"
	testFsPath   = "some_random_dir_path"
)

func BenchmarkHashFile(b *testing.B) {
	testFs := newDataFs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := HashFile(context.TODO(), "", testFs, testdataName, protocol.MinBlockSize, nil, true); err != nil {
			b.Fatal(err)
		}
	}

	b.SetBytes(testdataSize)
	b.ReportAllocs()
}

func newDataFs() fs.Filesystem {
	tfs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(16)+"?content=true")
	fd, err := tfs.Create(testdataName)
	if err != nil {
		panic(err)
	}

	lr := io.LimitReader(rand.Reader, testdataSize)
	if _, err := io.Copy(fd, lr); err != nil {
		panic(err)
	}

	if err := fd.Close(); err != nil {
		panic(err)
	}

	return tfs
}

func TestStopWalk(t *testing.T) {
	// Create tree that is 100 levels deep, with each level containing 100
	// files (each 1 MB) and 100 directories (in turn containing 100 files
	// and 100 directories, etc). That is, in total > 100^100 files and as
	// many directories. It'll take a while to scan, giving us time to
	// cancel it and make sure the scan stops.

	// Use an errorFs as the backing fs for the rest of the interface
	// The way we get it is a bit hacky tho.
	errorFs := fs.NewFilesystem(fs.FilesystemType(-1), ".")
	fs := fs.NewWalkFilesystem(&infiniteFS{errorFs, 100, 100, 1e6})

	const numHashers = 4
	ctx, cancel := context.WithCancel(context.Background())
	cfg, cfgCancel := testConfig()
	defer cfgCancel()
	cfg.Filesystem = fs
	cfg.Hashers = numHashers
	cfg.ProgressTickIntervalS = -1 // Don't attempt to build the full list of files before starting to scan...
	fchan := Walk(ctx, cfg)

	// Receive a few entries to make sure the walker is up and running,
	// scanning both files and dirs. Do some quick sanity tests on the
	// returned file entries to make sure we are not just reading crap from
	// a closed channel or something.
	dirs := 0
	files := 0
	for {
		res := <-fchan
		if res.Err != nil {
			t.Errorf("Error while scanning %v: %v", res.Err, res.Path)
		}
		f := res.File
		t.Log("Scanned", f)
		if f.IsDirectory() {
			if f.Name == "" || f.Permissions == 0 {
				t.Error("Bad directory entry", f)
			}
			dirs++
		} else {
			if f.Name == "" || len(f.Blocks) == 0 || f.Permissions == 0 {
				t.Error("Bad file entry", f)
			}
			files++
		}
		if dirs > 5 && files > 5 {
			break
		}
	}

	// Cancel the walker.
	cancel()

	// Empty out any waiting entries and wait for the channel to close.
	// Count them, they should be zero or very few - essentially, each
	// hasher has the choice of returning a fully handled entry or
	// cancelling, but they should not start on another item.
	extra := 0
	for range fchan {
		extra++
	}
	t.Log("Extra entries:", extra)
	if extra > numHashers {
		t.Error("unexpected extra entries received after cancel")
	}
}

func TestIssue4799(t *testing.T) {
	fs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(16))

	fd, err := fs.Create("foo")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	files := walkDir(fs, "/foo", nil, nil, 0)
	if len(files) != 1 || files[0].Name != "foo" {
		t.Error(`Received unexpected file infos when walking "/foo"`, files)
	}
}

func TestRecurseInclude(t *testing.T) {
	stignore := `
	!/dir1/cfile
	!efile
	!ffile
	*
	`
	testFs := newTestFs()
	ignores := ignore.New(testFs, ignore.WithCache(true))
	if err := ignores.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}

	files := walkDir(testFs, ".", nil, ignores, 0)

	expected := []string{
		filepath.Join("dir1"),
		filepath.Join("dir1", "cfile"),
		filepath.Join("dir2"),
		filepath.Join("dir2", "dir21"),
		filepath.Join("dir2", "dir21", "dir22"),
		filepath.Join("dir2", "dir21", "dir22", "dir23"),
		filepath.Join("dir2", "dir21", "dir22", "dir23", "efile"),
		filepath.Join("dir2", "dir21", "dir22", "efile"),
		filepath.Join("dir2", "dir21", "dir22", "efile", "efile"),
		filepath.Join("dir2", "dir21", "dira"),
		filepath.Join("dir2", "dir21", "dira", "efile"),
		filepath.Join("dir2", "dir21", "dira", "ffile"),
		filepath.Join("dir2", "dir21", "efile"),
		filepath.Join("dir2", "dir21", "efile", "ign"),
		filepath.Join("dir2", "dir21", "efile", "ign", "efile"),
	}
	if len(files) != len(expected) {
		var filesString []string
		for _, file := range files {
			filesString = append(filesString, file.Name)
		}
		t.Fatalf("Got %d files %v, expected %d files at %v", len(files), filesString, len(expected), expected)
	}
	for i := range files {
		if files[i].Name != expected[i] {
			t.Errorf("Got %v, expected file at %v", files[i], expected[i])
		}
	}
}

func TestIssue4841(t *testing.T) {
	fs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(16))

	fd, err := fs.Create("foo")
	if err != nil {
		panic(err)
	}
	fd.Close()

	cfg, cancel := testConfig()
	defer cancel()
	cfg.Filesystem = fs
	cfg.AutoNormalize = true
	cfg.CurrentFiler = fakeCurrentFiler{"foo": {
		Name:       "foo",
		Type:       protocol.FileInfoTypeFile,
		LocalFlags: protocol.FlagLocalIgnored,
		Version:    protocol.Vector{}.Update(1),
	}}
	cfg.ShortID = protocol.LocalDeviceID.Short()
	fchan := Walk(context.TODO(), cfg)

	var files []protocol.FileInfo
	for f := range fchan {
		if f.Err != nil {
			t.Errorf("Error while scanning %v: %v", f.Err, f.Path)
		}
		files = append(files, f.File)
	}
	sort.Sort(fileList(files))

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d: %v", len(files), files)
	}
	if expected := (protocol.Vector{}.Update(protocol.LocalDeviceID.Short())); !files[0].Version.Equal(expected) {
		t.Fatalf("Expected Version == %v, got %v", expected, files[0].Version)
	}
}

// TestNotExistingError reproduces https://github.com/syncthing/syncthing/issues/5385
func TestNotExistingError(t *testing.T) {
	sub := "notExisting"
	testFs := newTestFs()
	if _, err := testFs.Lstat(sub); !fs.IsNotExist(err) {
		t.Fatalf("Lstat returned error %v, while nothing should exist there.", err)
	}

	cfg, cancel := testConfig()
	defer cancel()
	cfg.Subs = []string{sub}
	fchan := Walk(context.TODO(), cfg)
	for f := range fchan {
		t.Fatalf("Expected no result from scan, got %v", f)
	}
}

func TestSkipIgnoredDirs(t *testing.T) {
	fss := fs.NewFilesystem(fs.FilesystemTypeFake, "")

	name := "foo/ignored"
	err := fss.MkdirAll(name, 0o777)
	if err != nil {
		t.Fatal(err)
	}

	stat, err := fss.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}

	w := &walker{}

	pats := ignore.New(fss, ignore.WithCache(true))

	stignore := `
	/foo/ign*
	!/f*
	*
	`
	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}
	if !pats.SkipIgnoredDirs() {
		t.Error("SkipIgnoredDirs should be true")
	}

	w.Matcher = pats

	fn := w.walkAndHashFiles(context.Background(), nil, nil)

	if err := fn(name, stat, nil); err != fs.SkipDir {
		t.Errorf("Expected %v, got %v", fs.SkipDir, err)
	}
}

// https://github.com/syncthing/syncthing/issues/6487
func TestIncludedSubdir(t *testing.T) {
	fss := fs.NewFilesystem(fs.FilesystemTypeFake, "")

	name := filepath.Clean("foo/bar/included")
	err := fss.MkdirAll(name, 0o777)
	if err != nil {
		t.Fatal(err)
	}

	pats := ignore.New(fss, ignore.WithCache(true))

	stignore := `
	!/foo/bar
	*
	`
	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}

	fchan := Walk(context.TODO(), Config{
		CurrentFiler: make(fakeCurrentFiler),
		Filesystem:   fss,
		Matcher:      pats,
	})

	found := false
	for f := range fchan {
		if f.Err != nil {
			t.Fatalf("Error while scanning %v: %v", f.Err, f.Path)
		}
		if f.File.IsIgnored() {
			t.Error("File is ignored:", f.File.Name)
		}
		if f.File.Name == name {
			found = true
		}
	}

	if !found {
		t.Errorf("File not present in scan results")
	}
}

// Verify returns nil or an error describing the mismatch between the block
// list and actual reader contents
func verify(r io.Reader, blocksize int, blocks []protocol.BlockInfo) error {
	hf := sha256.New()
	// A 32k buffer is used for copying into the hash function.
	buf := make([]byte, 32<<10)

	for i, block := range blocks {
		lr := &io.LimitedReader{R: r, N: int64(blocksize)}
		_, err := io.CopyBuffer(hf, lr, buf)
		if err != nil {
			return err
		}

		hash := hf.Sum(nil)
		hf.Reset()

		if !bytes.Equal(hash, block.Hash) {
			return fmt.Errorf("hash mismatch %x != %x for block %d", hash, block.Hash, i)
		}
	}

	// We should have reached the end  now
	bs := make([]byte, 1)
	n, err := r.Read(bs)
	if n != 0 || err != io.EOF {
		return errors.New("file continues past end of blocks")
	}

	return nil
}

type fakeCurrentFiler map[string]protocol.FileInfo

func (fcf fakeCurrentFiler) CurrentFile(name string) (protocol.FileInfo, bool) {
	f, ok := fcf[name]
	return f, ok
}

func testConfig() (Config, context.CancelFunc) {
	evLogger := events.NewLogger()
	ctx, cancel := context.WithCancel(context.Background())
	go evLogger.Serve(ctx)
	return Config{
		Filesystem:  newTestFs(),
		Hashers:     2,
		EventLogger: evLogger,
	}, cancel
}

func BenchmarkWalk(b *testing.B) {
	testFs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32))

	for i := 0; i < 100; i++ {
		if err := testFs.Mkdir(fmt.Sprintf("dir%d", i), 0o755); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 100; j++ {
			if fd, err := testFs.Create(fmt.Sprintf("dir%d/file%d", i, j)); err != nil {
				b.Fatal(err)
			} else {
				fd.Close()
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		walkDir(testFs, "/", nil, nil, 0)
	}
}
