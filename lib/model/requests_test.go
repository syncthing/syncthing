// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestRequestSimple(t *testing.T) {
	// Verify that the model performs a request and creates a file based on
	// an incoming index update.

	m, fc, fcfg := setupModelWithConnection()
	tfs := fcfg.Filesystem()
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("More than one index update sent")
		default:
		}
		for _, f := range fs {
			if f.Name == "testfile" {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	contents := []byte("test file contents\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, contents)
	fc.sendIndexUpdate()
	<-done

	// Verify the contents
	if err := equalContents(filepath.Join(tfs.URI(), "testfile"), contents); err != nil {
		t.Error("File did not sync correctly:", err)
	}
}

func TestSymlinkTraversalRead(t *testing.T) {
	// Verify that a symlink can not be traversed for reading.

	if runtime.GOOS == "windows" {
		t.Skip("no symlink support on CI")
		return
	}

	m, fc, fcfg := setupModelWithConnection()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem().URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("More than one index update sent")
		default:
		}
		for _, f := range fs {
			if f.Name == "symlink" {
				close(done)
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the symlink, wait for it to sync and be reported back.
	contents := []byte("..")
	fc.addFile("symlink", 0644, protocol.FileInfoTypeSymlink, contents)
	fc.sendIndexUpdate()
	<-done

	// Request a file by traversing the symlink
	res, err := m.Request(device1, "default", "symlink/requests_test.go", 10, 0, nil, 0, false)
	if err == nil || res != nil {
		t.Error("Managed to traverse symlink")
	}
}

func TestSymlinkTraversalWrite(t *testing.T) {
	// Verify that a symlink can not be traversed for writing.

	if runtime.GOOS == "windows" {
		t.Skip("no symlink support on CI")
		return
	}

	m, fc, fcfg := setupModelWithConnection()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem().URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected names.
	done := make(chan struct{}, 1)
	badReq := make(chan string, 1)
	badIdx := make(chan string, 1)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if f.Name == "symlink" {
				done <- struct{}{}
				return
			}
			if strings.HasPrefix(f.Name, "symlink") {
				badIdx <- f.Name
				return
			}
		}
	}
	fc.requestFn = func(folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error) {
		if name != "symlink" && strings.HasPrefix(name, "symlink") {
			badReq <- name
		}
		return fc.fileData[name], nil
	}
	fc.mut.Unlock()

	// Send an update for the symlink, wait for it to sync and be reported back.
	contents := []byte("..")
	fc.addFile("symlink", 0644, protocol.FileInfoTypeSymlink, contents)
	fc.sendIndexUpdate()
	<-done

	// Send an update for things behind the symlink, wait for requests for
	// blocks for any of them to come back, or index entries. Hopefully none
	// of that should happen.
	contents = []byte("testdata testdata\n")
	fc.addFile("symlink/testfile", 0644, protocol.FileInfoTypeFile, contents)
	fc.addFile("symlink/testdir", 0644, protocol.FileInfoTypeDirectory, contents)
	fc.addFile("symlink/testsyml", 0644, protocol.FileInfoTypeSymlink, contents)
	fc.sendIndexUpdate()

	select {
	case name := <-badReq:
		t.Fatal("Should not have requested the data for", name)
	case name := <-badIdx:
		t.Fatal("Should not have sent the index entry for", name)
	case <-time.After(3 * time.Second):
		// Unfortunately not much else to trigger on here. The puller sleep
		// interval is 1s so if we didn't get any requests within two
		// iterations we should be fine.
	}
}

func TestRequestCreateTmpSymlink(t *testing.T) {
	// Test that an update for a temporary file is invalidated

	m, fc, fcfg := setupModelWithConnection()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem().URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	goodIdx := make(chan struct{})
	name := fs.TempName("testlink")
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		for _, f := range fs {
			if f.Name == name {
				if f.IsInvalid() {
					goodIdx <- struct{}{}
				} else {
					t.Fatal("Received index with non-invalid temporary file")
				}
				return
			}
		}
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	fc.addFile(name, 0644, protocol.FileInfoTypeSymlink, []byte(".."))
	fc.sendIndexUpdate()

	select {
	case <-goodIdx:
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out without index entry being sent")
	}
}

func TestRequestVersioningSymlinkAttack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no symlink support on Windows")
	}

	// Sets up a folder with trashcan versioning and tries to use a
	// deleted symlink to escape

	w, fcfg := tmpDefaultWrapper()
	defer func() {
		os.RemoveAll(fcfg.Filesystem().URI())
		os.Remove(w.ConfigPath())
	}()

	fcfg.Versioning = config.VersioningConfiguration{Type: "trashcan"}
	w.SetFolder(fcfg)
	m, fc := setupModelWithConnectionFromWrapper(w)
	defer cleanupModel(m)

	// Create a temporary directory that we will use as target to see if
	// we can escape to it
	tmpdir, err := ioutil.TempDir("", "syncthing-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	idx := make(chan int)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		idx <- len(fs)
	}
	fc.mut.Unlock()

	// Send an update for the test file, wait for it to sync and be reported back.
	fc.addFile("foo", 0644, protocol.FileInfoTypeSymlink, []byte(tmpdir))
	fc.sendIndexUpdate()

	for updates := 0; updates < 1; updates += <-idx {
	}

	// Delete the symlink, hoping for it to get versioned
	fc.deleteFile("foo")
	fc.sendIndexUpdate()
	for updates := 0; updates < 1; updates += <-idx {
	}

	// Recreate foo and a file in it with some data
	fc.updateFile("foo", 0755, protocol.FileInfoTypeDirectory, nil)
	fc.addFile("foo/test", 0644, protocol.FileInfoTypeFile, []byte("testtesttest"))
	fc.sendIndexUpdate()
	for updates := 0; updates < 1; updates += <-idx {
	}

	// Remove the test file and see if it escaped
	fc.deleteFile("foo/test")
	fc.sendIndexUpdate()
	for updates := 0; updates < 1; updates += <-idx {
	}

	path := filepath.Join(tmpdir, "test")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("File escaped to", path)
	}
}

func TestPullInvalidIgnoredSO(t *testing.T) {
	pullInvalidIgnored(t, config.FolderTypeSendOnly)

}

func TestPullInvalidIgnoredSR(t *testing.T) {
	pullInvalidIgnored(t, config.FolderTypeSendReceive)
}

// This test checks that (un-)ignored/invalid/deleted files are treated as expected.
func pullInvalidIgnored(t *testing.T, ft config.FolderType) {
	w := createTmpWrapper(defaultCfgWrapper.RawCopy())
	fcfg := testFolderConfigTmp()
	fss := fcfg.Filesystem()
	fcfg.Type = ft
	w.SetFolder(fcfg)
	m := setupModel(w)
	defer cleanupModelAndRemoveDir(m, fss.URI())

	m.RemoveFolder(fcfg)
	m.AddFolder(fcfg)
	// Reach in and update the ignore matcher to one that always does
	// reloads when asked to, instead of checking file mtimes. This is
	// because we might be changing the files on disk often enough that the
	// mtimes will be unreliable to determine change status.
	m.fmut.Lock()
	m.folderIgnores["default"] = ignore.New(fss, ignore.WithChangeDetector(newAlwaysChanged()))
	m.fmut.Unlock()
	m.StartFolder(fcfg.ID)

	fc := addFakeConn(m, device1)
	fc.folder = "default"

	if err := m.SetIgnores("default", []string{"*ignored*"}); err != nil {
		panic(err)
	}

	contents := []byte("test file contents\n")
	otherContents := []byte("other test file contents\n")

	invIgn := "invalid:ignored"
	invDel := "invalid:deleted"
	ign := "ignoredNonExisting"
	ignExisting := "ignoredExisting"

	fc.addFile(invIgn, 0644, protocol.FileInfoTypeFile, contents)
	fc.addFile(invDel, 0644, protocol.FileInfoTypeFile, contents)
	fc.deleteFile(invDel)
	fc.addFile(ign, 0644, protocol.FileInfoTypeFile, contents)
	fc.addFile(ignExisting, 0644, protocol.FileInfoTypeFile, contents)
	if err := ioutil.WriteFile(filepath.Join(fss.URI(), ignExisting), otherContents, 0644); err != nil {
		panic(err)
	}

	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		expected := map[string]struct{}{invIgn: {}, ign: {}, ignExisting: {}}
		for _, f := range fs {
			if _, ok := expected[f.Name]; !ok {
				t.Errorf("Unexpected file %v was added to index", f.Name)
			}
			if !f.IsInvalid() {
				t.Errorf("File %v wasn't marked as invalid", f.Name)
			}
			delete(expected, f.Name)
		}
		for name := range expected {
			t.Errorf("File %v wasn't added to index", name)
		}
		close(done)
	}
	fc.mut.Unlock()

	sub := events.Default.Subscribe(events.FolderErrors)
	defer events.Default.Unsubscribe(sub)

	fc.sendIndexUpdate()

	select {
	case ev := <-sub.C():
		t.Fatalf("Errors while scanning/pulling: %v", ev)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out before index was received")
	case <-done:
	}

	done = make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		expected := map[string]struct{}{ign: {}, ignExisting: {}}
		for _, f := range fs {
			if _, ok := expected[f.Name]; !ok {
				t.Fatalf("Unexpected file %v was updated in index", f.Name)
			}
			if f.IsInvalid() {
				t.Errorf("File %v is still marked as invalid", f.Name)
			}
			// The unignored files should only have a local version,
			// to mark them as in conflict with any other existing versions.
			ev := protocol.Vector{}.Update(myID.Short())
			if v := f.Version; !v.Equal(ev) {
				t.Errorf("File %v has version %v, expected %v", f.Name, v, ev)
			}
			if f.Name == ign {
				if !f.Deleted {
					t.Errorf("File %v was not marked as deleted", f.Name)
				}
			} else if f.Deleted {
				t.Errorf("File %v is marked as deleted", f.Name)
			}
			delete(expected, f.Name)
		}
		for name := range expected {
			t.Errorf("File %v wasn't updated in index", name)
		}
		close(done)
	}
	// Make sure pulling doesn't interfere, as index updates are racy and
	// thus we cannot distinguish between scan and pull results.
	fc.requestFn = func(folder, name string, offset int64, size int, hash []byte, fromTemporary bool) ([]byte, error) {
		return nil, nil
	}
	fc.mut.Unlock()

	if err := m.SetIgnores("default", []string{"*:ignored*"}); err != nil {
		panic(err)
	}

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out before index was received")
	case <-done:
	}
}

func TestIssue4841(t *testing.T) {
	m, fc, fcfg := setupModelWithConnection()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem().URI())

	received := make(chan protocol.FileInfo)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		if len(fs) != 1 {
			t.Fatalf("Sent index with %d files, should be 1", len(fs))
		}
		if fs[0].Name != "foo" {
			t.Fatalf(`Sent index with file %v, should be "foo"`, fs[0].Name)
		}
		received <- fs[0]
	}
	fc.mut.Unlock()

	// Setup file from remote that was ignored locally
	folder := m.folderRunners[defaultFolderConfig.ID].(*sendReceiveFolder)
	folder.updateLocals([]protocol.FileInfo{{
		Name:       "foo",
		Type:       protocol.FileInfoTypeFile,
		LocalFlags: protocol.FlagLocalIgnored,
		Version:    protocol.Vector{}.Update(device1.Short()),
	}})
	<-received

	// Scan without ignore patterns with "foo" not existing locally
	if err := m.ScanFolder("default"); err != nil {
		t.Fatal("Failed scanning:", err)
	}

	f := <-received
	if expected := (protocol.Vector{}.Update(myID.Short())); !f.Version.Equal(expected) {
		t.Errorf("Got Version == %v, expected %v", f.Version, expected)
	}
}

func TestRescanIfHaveInvalidContent(t *testing.T) {
	m, fc, fcfg := setupModelWithConnection()
	tmpDir := fcfg.Filesystem().URI()
	defer cleanupModelAndRemoveDir(m, tmpDir)

	payload := []byte("hello")

	must(t, ioutil.WriteFile(filepath.Join(tmpDir, "foo"), payload, 0777))

	received := make(chan protocol.FileInfo)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		if len(fs) != 1 {
			t.Fatalf("Sent index with %d files, should be 1", len(fs))
		}
		if fs[0].Name != "foo" {
			t.Fatalf(`Sent index with file %v, should be "foo"`, fs[0].Name)
		}
		received <- fs[0]
	}
	fc.mut.Unlock()

	// Scan without ignore patterns with "foo" not existing locally
	if err := m.ScanFolder("default"); err != nil {
		t.Fatal("Failed scanning:", err)
	}

	f := <-received
	if f.Blocks[0].WeakHash != 103547413 {
		t.Fatalf("unexpected weak hash: %d != 103547413", f.Blocks[0].WeakHash)
	}

	res, err := m.Request(device1, "default", "foo", int32(len(payload)), 0, f.Blocks[0].Hash, f.Blocks[0].WeakHash, false)
	if err != nil {
		t.Fatal(err)
	}
	buf := res.Data()
	if !bytes.Equal(buf, payload) {
		t.Errorf("%s != %s", buf, payload)
	}

	payload = []byte("bye")
	buf = make([]byte, len(payload))

	must(t, ioutil.WriteFile(filepath.Join(tmpDir, "foo"), payload, 0777))

	_, err = m.Request(device1, "default", "foo", int32(len(payload)), 0, f.Blocks[0].Hash, f.Blocks[0].WeakHash, false)
	if err == nil {
		t.Fatalf("expected failure")
	}

	select {
	case f := <-received:
		if f.Blocks[0].WeakHash != 41943361 {
			t.Fatalf("unexpected weak hash: %d != 41943361", f.Blocks[0].WeakHash)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out")
	}
}

func TestParentDeletion(t *testing.T) {
	m, fc, fcfg := setupModelWithConnection()
	testFs := fcfg.Filesystem()
	defer cleanupModelAndRemoveDir(m, testFs.URI())

	parent := "foo"
	child := filepath.Join(parent, "bar")

	received := make(chan []protocol.FileInfo)
	fc.addFile(parent, 0777, protocol.FileInfoTypeDirectory, nil)
	fc.addFile(child, 0777, protocol.FileInfoTypeDirectory, nil)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		received <- fs
	}
	fc.mut.Unlock()
	fc.sendIndexUpdate()

	// Get back index from initial setup
	select {
	case fs := <-received:
		if len(fs) != 2 {
			t.Fatalf("Sent index with %d files, should be 2", len(fs))
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out")
	}

	// Delete parent dir
	must(t, testFs.RemoveAll(parent))

	// Scan only the child dir (not the parent)
	if err := m.ScanFolderSubdirs("default", []string{child}); err != nil {
		t.Fatal("Failed scanning:", err)
	}

	select {
	case fs := <-received:
		if len(fs) != 1 {
			t.Fatalf("Sent index with %d files, should be 1", len(fs))
		}
		if fs[0].Name != child {
			t.Fatalf(`Sent index with file "%v", should be "%v"`, fs[0].Name, child)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out")
	}

	// Recreate the child dir on the remote
	fc.updateFile(child, 0777, protocol.FileInfoTypeDirectory, nil)
	fc.sendIndexUpdate()

	// Wait for the child dir to be recreated and sent to the remote
	select {
	case fs := <-received:
		l.Debugln("sent:", fs)
		found := false
		for _, f := range fs {
			if f.Name == child {
				if f.Deleted {
					t.Fatalf(`File "%v" is still deleted`, child)
				}
				found = true
			}
		}
		if !found {
			t.Fatalf(`File "%v" is missing in index`, child)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out")
	}
}

// TestRequestSymlinkWindows checks that symlinks aren't marked as deleted on windows
// Issue: https://github.com/syncthing/syncthing/issues/5125
func TestRequestSymlinkWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows specific test")
	}

	m, fc, fcfg := setupModelWithConnection()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem().URI())

	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("More than one index update sent")
		default:
		}
		// expected first index
		if len(fs) != 1 {
			t.Fatalf("Expected just one file in index, got %v", fs)
		}
		f := fs[0]
		if f.Name != "link" {
			t.Fatalf(`Got file info with path "%v", expected "link"`, f.Name)
		}
		if !f.IsInvalid() {
			t.Errorf(`File info was not marked as invalid`)
		}
		close(done)
	}
	fc.mut.Unlock()

	fc.addFile("link", 0644, protocol.FileInfoTypeSymlink, nil)
	fc.sendIndexUpdate()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timed out before pull was finished")
	}

	sub := events.Default.Subscribe(events.StateChanged | events.LocalIndexUpdated)
	defer events.Default.Unsubscribe(sub)

	m.ScanFolder("default")

	for {
		select {
		case ev := <-sub.C():
			switch data := ev.Data.(map[string]interface{}); {
			case ev.Type == events.LocalIndexUpdated:
				t.Fatalf("Local index was updated unexpectedly: %v", data)
			case ev.Type == events.StateChanged:
				if data["from"] == "scanning" {
					return
				}
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Timed out before scan finished")
		}
	}
}

func equalContents(path string, contents []byte) error {
	if bs, err := ioutil.ReadFile(path); err != nil {
		return err
	} else if !bytes.Equal(bs, contents) {
		return errors.New("incorrect data")
	}
	return nil
}

func TestRequestRemoteRenameChanged(t *testing.T) {
	m, fc, fcfg := setupModelWithConnection()
	tfs := fcfg.Filesystem()
	tmpDir := tfs.URI()
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("More than one index update sent")
		default:
		}
		if len(fs) != 2 {
			t.Fatalf("Received index with %v indexes instead of 2", len(fs))
		}
		close(done)
	}
	fc.mut.Unlock()

	// setup
	a := "a"
	b := "b"
	data := map[string][]byte{
		a: []byte("aData"),
		b: []byte("bData"),
	}
	for _, n := range [2]string{a, b} {
		fc.addFile(n, 0644, protocol.FileInfoTypeFile, data[n])
	}
	fc.sendIndexUpdate()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	for _, n := range [2]string{a, b} {
		must(t, equalContents(filepath.Join(tmpDir, n), data[n]))
	}

	var gotA, gotB, gotConfl bool
	done = make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("Received more index updates than expected")
		default:
		}
		for _, f := range fs {
			switch {
			case f.Name == a:
				if gotA {
					t.Error("Got more than one index update for", f.Name)
				}
				gotA = true
			case f.Name == b:
				if gotB {
					t.Error("Got more than one index update for", f.Name)
				}
				gotB = true
			case strings.HasPrefix(f.Name, "b.sync-conflict-"):
				if gotConfl {
					t.Error("Got more than one index update for conflicts of", f.Name)
				}
				gotConfl = true
			default:
				t.Error("Got unexpected file in index update", f.Name)
			}
		}
		if gotA && gotB && gotConfl {
			close(done)
		}
	}
	fc.mut.Unlock()

	fd, err := tfs.OpenFile(b, fs.OptReadWrite, 0644)
	if err != nil {
		t.Fatal(err)
	}
	otherData := []byte("otherData")
	if _, err = fd.Write(otherData); err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// rename
	fc.deleteFile(a)
	fc.updateFile(b, 0644, protocol.FileInfoTypeFile, data[a])
	// Make sure the remote file for b is newer and thus stays global -> local conflict
	fc.mut.Lock()
	for i := range fc.files {
		if fc.files[i].Name == b {
			fc.files[i].ModifiedS += 100
			break
		}
	}
	fc.mut.Unlock()
	fc.sendIndexUpdate()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Errorf("timed out without receiving all expected index updates")
	}

	// Check outcome
	tfs.Walk(".", func(path string, info fs.FileInfo, err error) error {
		switch {
		case path == a:
			t.Errorf(`File "a" was not removed`)
		case path == b:
			if err := equalContents(filepath.Join(tmpDir, b), data[a]); err != nil {
				t.Error(`File "b" has unexpected content (renamed from a on remote)`)
			}
		case strings.HasPrefix(path, b+".sync-conflict-"):
			if err := equalContents(filepath.Join(tmpDir, path), otherData); err != nil {
				t.Error(`Sync conflict of "b" has unexptected content`)
			}
		case path == "." || strings.HasPrefix(path, ".stfolder"):
		default:
			t.Error("Found unexpected file", path)
		}
		return nil
	})
}

func TestRequestRemoteRenameConflict(t *testing.T) {
	m, fc, fcfg := setupModelWithConnection()
	tfs := fcfg.Filesystem()
	tmpDir := tfs.URI()
	defer cleanupModelAndRemoveDir(m, tmpDir)

	recv := make(chan int)
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		recv <- len(fs)
	}
	fc.mut.Unlock()

	// setup
	a := "a"
	b := "b"
	data := map[string][]byte{
		a: []byte("aData"),
		b: []byte("bData"),
	}
	for _, n := range [2]string{a, b} {
		fc.addFile(n, 0644, protocol.FileInfoTypeFile, data[n])
	}
	fc.sendIndexUpdate()
	select {
	case i := <-recv:
		if i != 2 {
			t.Fatalf("received %v items in index, expected 1", i)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	for _, n := range [2]string{a, b} {
		must(t, equalContents(filepath.Join(tmpDir, n), data[n]))
	}

	fd, err := tfs.OpenFile(b, fs.OptReadWrite, 0644)
	if err != nil {
		t.Fatal(err)
	}
	otherData := []byte("otherData")
	if _, err = fd.Write(otherData); err != nil {
		t.Fatal(err)
	}
	fd.Close()
	m.ScanFolders()
	select {
	case i := <-recv:
		if i != 1 {
			t.Fatalf("received %v items in index, expected 1", i)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	// make sure the following rename is more recent (not concurrent)
	time.Sleep(2 * time.Second)

	// rename
	fc.deleteFile(a)
	fc.updateFile(b, 0644, protocol.FileInfoTypeFile, data[a])
	fc.sendIndexUpdate()
	select {
	case <-recv:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	// Check outcome
	foundB := false
	foundBConfl := false
	tfs.Walk(".", func(path string, info fs.FileInfo, err error) error {
		switch {
		case path == a:
			t.Errorf(`File "a" was not removed`)
		case path == b:
			foundB = true
		case strings.HasPrefix(path, b) && strings.Contains(path, ".sync-conflict-"):
			foundBConfl = true
		}
		return nil
	})
	if !foundB {
		t.Errorf(`File "b" was removed`)
	}
	if !foundBConfl {
		t.Errorf(`No conflict file for "b" was created`)
	}
}

func TestRequestDeleteChanged(t *testing.T) {
	m, fc, fcfg := setupModelWithConnection()
	tfs := fcfg.Filesystem()
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	done := make(chan struct{})
	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("More than one index update sent")
		default:
		}
		close(done)
	}
	fc.mut.Unlock()

	// setup
	a := "a"
	data := []byte("aData")
	fc.addFile(a, 0644, protocol.FileInfoTypeFile, data)
	fc.sendIndexUpdate()
	select {
	case <-done:
		done = make(chan struct{})
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	fc.mut.Lock()
	fc.indexFn = func(folder string, fs []protocol.FileInfo) {
		select {
		case <-done:
			t.Fatalf("More than one index update sent")
		default:
		}
		close(done)
	}
	fc.mut.Unlock()

	fd, err := tfs.OpenFile(a, fs.OptReadWrite, 0644)
	if err != nil {
		t.Fatal(err)
	}
	otherData := []byte("otherData")
	if _, err = fd.Write(otherData); err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// rename
	fc.deleteFile(a)
	fc.sendIndexUpdate()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	// Check outcome
	if _, err := tfs.Lstat(a); err != nil {
		if fs.IsNotExist(err) {
			t.Error(`Modified file "a" was removed`)
		} else {
			t.Error(`Error stating file "a":`, err)
		}
	}
}
