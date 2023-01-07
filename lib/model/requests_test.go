// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

func TestRequestSimple(t *testing.T) {
	// Verify that the model performs a request and creates a file based on
	// an incoming index update.

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-done:
			t.Error("More than one index update sent")
		default:
		}
		for _, f := range fs {
			if f.Name == "testfile" {
				close(done)
				return nil
			}
		}
		return nil
	})

	// Send an update for the test file, wait for it to sync and be reported back.
	contents := []byte("test file contents\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, contents)
	fc.sendIndexUpdate()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	// Verify the contents
	if err := equalContents(filepath.Join(tfs.URI(), "testfile"), contents); err != nil {
		t.Error("File did not sync correctly:", err)
	}
}

func TestSymlinkTraversalRead(t *testing.T) {
	// Verify that a symlink can not be traversed for reading.

	if build.IsWindows {
		t.Skip("no symlink support on CI")
		return
	}

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	done := make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-done:
			t.Error("More than one index update sent")
		default:
		}
		for _, f := range fs {
			if f.Name == "symlink" {
				close(done)
				return nil
			}
		}
		return nil
	})

	// Send an update for the symlink, wait for it to sync and be reported back.
	contents := []byte("..")
	fc.addFile("symlink", 0644, protocol.FileInfoTypeSymlink, contents)
	fc.sendIndexUpdate()
	<-done

	// Request a file by traversing the symlink
	res, err := m.Request(device1, "default", "symlink/requests_test.go", 0, 10, 0, nil, 0, false)
	if err == nil || res != nil {
		t.Error("Managed to traverse symlink")
	}
}

func TestSymlinkTraversalWrite(t *testing.T) {
	// Verify that a symlink can not be traversed for writing.

	if build.IsWindows {
		t.Skip("no symlink support on CI")
		return
	}

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected names.
	done := make(chan struct{}, 1)
	badReq := make(chan string, 1)
	badIdx := make(chan string, 1)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		for _, f := range fs {
			if f.Name == "symlink" {
				done <- struct{}{}
				return nil
			}
			if strings.HasPrefix(f.Name, "symlink") {
				badIdx <- f.Name
				return nil
			}
		}
		return nil
	})
	fc.RequestCalls(func(ctx context.Context, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
		if name != "symlink" && strings.HasPrefix(name, "symlink") {
			badReq <- name
		}
		return fc.fileData[name], nil
	})

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

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	goodIdx := make(chan struct{})
	name := fs.TempName("testlink")
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		for _, f := range fs {
			if f.Name == name {
				if f.IsInvalid() {
					goodIdx <- struct{}{}
				} else {
					t.Error("Received index with non-invalid temporary file")
					close(goodIdx)
				}
				return nil
			}
		}
		return nil
	})

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
	if build.IsWindows {
		t.Skip("no symlink support on Windows")
	}

	// Sets up a folder with trashcan versioning and tries to use a
	// deleted symlink to escape

	w, fcfg, wCancel := tmpDefaultWrapper(t)
	defer wCancel()
	defer func() {
		os.RemoveAll(fcfg.Filesystem(nil).URI())
		os.Remove(w.ConfigPath())
	}()

	fcfg.Versioning = config.VersioningConfiguration{Type: "trashcan"}
	setFolder(t, w, fcfg)
	m, fc := setupModelWithConnectionFromWrapper(t, w)
	defer cleanupModel(m)

	// Create a temporary directory that we will use as target to see if
	// we can escape to it
	tmpdir := t.TempDir()

	// We listen for incoming index updates and trigger when we see one for
	// the expected test file.
	idx := make(chan int)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		idx <- len(fs)
		return nil
	})

	waitForIdx := func() {
		select {
		case c := <-idx:
			if c == 0 {
				t.Fatal("Got empty index update")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out before receiving index update")
		}
	}

	// Send an update for the test file, wait for it to sync and be reported back.
	fc.addFile("foo", 0644, protocol.FileInfoTypeSymlink, []byte(tmpdir))
	fc.sendIndexUpdate()
	waitForIdx()

	// Delete the symlink, hoping for it to get versioned
	fc.deleteFile("foo")
	fc.sendIndexUpdate()
	waitForIdx()

	// Recreate foo and a file in it with some data
	fc.updateFile("foo", 0755, protocol.FileInfoTypeDirectory, nil)
	fc.addFile("foo/test", 0644, protocol.FileInfoTypeFile, []byte("testtesttest"))
	fc.sendIndexUpdate()
	waitForIdx()

	// Remove the test file and see if it escaped
	fc.deleteFile("foo/test")
	fc.sendIndexUpdate()
	waitForIdx()

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
	w, wCancel := createTmpWrapper(defaultCfgWrapper.RawCopy())
	defer wCancel()
	fcfg := testFolderConfig(t.TempDir())
	fss := fcfg.Filesystem(nil)
	fcfg.Type = ft
	setFolder(t, w, fcfg)
	m := setupModel(t, w)
	defer cleanupModelAndRemoveDir(m, fss.URI())

	folderIgnoresAlwaysReload(t, m, fcfg)

	fc := addFakeConn(m, device1, fcfg.ID)
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
	writeFile(t, fss, ignExisting, otherContents)

	done := make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
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
		return nil
	})

	sub := m.evLogger.Subscribe(events.FolderErrors)
	defer sub.Unsubscribe()

	fc.sendIndexUpdate()

	select {
	case ev := <-sub.C():
		t.Fatalf("Errors while scanning/pulling: %v", ev)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out before index was received")
	case <-done:
	}

	done = make(chan struct{})
	expected := map[string]struct{}{ign: {}, ignExisting: {}}
	var expectedMut sync.Mutex
	// The indexes will normally arrive in one update, but it is possible
	// that they arrive in separate ones.
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		expectedMut.Lock()
		for _, f := range fs {
			_, ok := expected[f.Name]
			if !ok {
				t.Errorf("Unexpected file %v was updated in index", f.Name)
				continue
			}
			if f.IsInvalid() {
				t.Errorf("File %v is still marked as invalid", f.Name)
			}
			if f.Name == ign {
				// The unignored deleted file should have an
				// empty version, to make it not override
				// existing global files.
				if !f.Deleted {
					t.Errorf("File %v was not marked as deleted", f.Name)
				}
				if len(f.Version.Counters) != 0 {
					t.Errorf("File %v has version %v, expected empty", f.Name, f.Version)
				}
			} else {
				// The unignored existing file should have a
				// version with only a local counter, to make
				// it conflict changed global files.
				if f.Deleted {
					t.Errorf("File %v is marked as deleted", f.Name)
				}
				if len(f.Version.Counters) != 1 || f.Version.Counter(myID.Short()) == 0 {
					t.Errorf("File %v has version %v, expected one entry for ourselves", f.Name, f.Version)
				}
			}
			delete(expected, f.Name)
		}
		if len(expected) == 0 {
			close(done)
		}
		expectedMut.Unlock()
		return nil
	})
	// Make sure pulling doesn't interfere, as index updates are racy and
	// thus we cannot distinguish between scan and pull results.
	fc.RequestCalls(func(ctx context.Context, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
		return nil, nil
	})

	if err := m.SetIgnores("default", []string{"*:ignored*"}); err != nil {
		panic(err)
	}

	select {
	case <-time.After(5 * time.Second):
		expectedMut.Lock()
		t.Fatal("timed out before receiving index updates for all existing files, missing", expected)
		expectedMut.Unlock()
	case <-done:
	}
}

func TestIssue4841(t *testing.T) {
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	received := make(chan []protocol.FileInfo)
	fc.setIndexFn(func(_ context.Context, _ string, fs []protocol.FileInfo) error {
		received <- fs
		return nil
	})
	checkReceived := func(fs []protocol.FileInfo) protocol.FileInfo {
		t.Helper()
		if len(fs) != 1 {
			t.Fatalf("Sent index with %d files, should be 1", len(fs))
		}
		if fs[0].Name != "foo" {
			t.Fatalf(`Sent index with file %v, should be "foo"`, fs[0].Name)
		}
		return fs[0]
	}

	// Setup file from remote that was ignored locally
	folder := m.folderRunners[defaultFolderConfig.ID].(*sendReceiveFolder)
	folder.updateLocals([]protocol.FileInfo{{
		Name:       "foo",
		Type:       protocol.FileInfoTypeFile,
		LocalFlags: protocol.FlagLocalIgnored,
		Version:    protocol.Vector{}.Update(device1.Short()),
	}})

	checkReceived(<-received)

	// Scan without ignore patterns with "foo" not existing locally
	if err := m.ScanFolder("default"); err != nil {
		t.Fatal("Failed scanning:", err)
	}

	select {
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	case r := <-received:
		f := checkReceived(r)
		if !f.Version.Equal(protocol.Vector{}) {
			t.Errorf("Got Version == %v, expected empty version", f.Version)
		}
	}
}

func TestRescanIfHaveInvalidContent(t *testing.T) {
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	payload := []byte("hello")

	writeFile(t, tfs, "foo", payload)

	received := make(chan []protocol.FileInfo)
	fc.setIndexFn(func(_ context.Context, _ string, fs []protocol.FileInfo) error {
		received <- fs
		return nil
	})
	checkReceived := func(fs []protocol.FileInfo) protocol.FileInfo {
		t.Helper()
		if len(fs) != 1 {
			t.Fatalf("Sent index with %d files, should be 1", len(fs))
		}
		if fs[0].Name != "foo" {
			t.Fatalf(`Sent index with file %v, should be "foo"`, fs[0].Name)
		}
		return fs[0]
	}

	// Scan without ignore patterns with "foo" not existing locally
	if err := m.ScanFolder("default"); err != nil {
		t.Fatal("Failed scanning:", err)
	}

	f := checkReceived(<-received)
	if f.Blocks[0].WeakHash != 103547413 {
		t.Fatalf("unexpected weak hash: %d != 103547413", f.Blocks[0].WeakHash)
	}

	res, err := m.Request(device1, "default", "foo", 0, int32(len(payload)), 0, f.Blocks[0].Hash, f.Blocks[0].WeakHash, false)
	if err != nil {
		t.Fatal(err)
	}
	buf := res.Data()
	if !bytes.Equal(buf, payload) {
		t.Errorf("%s != %s", buf, payload)
	}

	payload = []byte("bye")
	buf = make([]byte, len(payload))

	writeFile(t, tfs, "foo", payload)

	_, err = m.Request(device1, "default", "foo", 0, int32(len(payload)), 0, f.Blocks[0].Hash, f.Blocks[0].WeakHash, false)
	if err == nil {
		t.Fatalf("expected failure")
	}

	select {
	case fs := <-received:
		f := checkReceived(fs)
		if f.Blocks[0].WeakHash != 41943361 {
			t.Fatalf("unexpected weak hash: %d != 41943361", f.Blocks[0].WeakHash)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out")
	}
}

func TestParentDeletion(t *testing.T) {
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	testFs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, testFs.URI())

	parent := "foo"
	child := filepath.Join(parent, "bar")

	received := make(chan []protocol.FileInfo)
	fc.addFile(parent, 0777, protocol.FileInfoTypeDirectory, nil)
	fc.addFile(child, 0777, protocol.FileInfoTypeDirectory, nil)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		received <- fs
		return nil
	})
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
	if !build.IsWindows {
		t.Skip("windows specific test")
	}

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	defer cleanupModelAndRemoveDir(m, fcfg.Filesystem(nil).URI())

	received := make(chan []protocol.FileInfo)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-received:
			t.Error("More than one index update sent")
		default:
		}
		received <- fs
		return nil
	})

	fc.addFile("link", 0644, protocol.FileInfoTypeSymlink, nil)
	fc.sendIndexUpdate()

	select {
	case fs := <-received:
		close(received)
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
	case <-time.After(time.Second):
		t.Fatalf("timed out before pull was finished")
	}

	sub := m.evLogger.Subscribe(events.StateChanged | events.LocalIndexUpdated)
	defer sub.Unsubscribe()

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
	if bs, err := os.ReadFile(path); err != nil {
		return err
	} else if !bytes.Equal(bs, contents) {
		return errors.New("incorrect data")
	}
	return nil
}

func TestRequestRemoteRenameChanged(t *testing.T) {
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	tmpDir := tfs.URI()
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	received := make(chan []protocol.FileInfo)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-received:
			t.Error("More than one index update sent")
		default:
		}
		received <- fs
		return nil
	})

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
	case fs := <-received:
		close(received)
		if len(fs) != 2 {
			t.Fatalf("Received index with %v indexes instead of 2", len(fs))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	for _, n := range [2]string{a, b} {
		must(t, equalContents(filepath.Join(tmpDir, n), data[n]))
	}

	var gotA, gotB, gotConfl bool
	done := make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-done:
			t.Error("Received more index updates than expected")
			return nil
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
				if f.Version.Counter(fc.id.Short()) == 0 {
					// This index entry might be superseeded
					// by the final one or sent before it separately.
					break
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
		return nil
	})

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
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	tmpDir := tfs.URI()
	defer cleanupModelAndRemoveDir(m, tmpDir)

	recv := make(chan int)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		recv <- len(fs)
		return nil
	})

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
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	done := make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-done:
			t.Error("More than one index update sent")
		default:
		}
		close(done)
		return nil
	})

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

	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case <-done:
			t.Error("More than one index update sent")
		default:
		}
		close(done)
		return nil
	})

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

func TestNeedFolderFiles(t *testing.T) {
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	tmpDir := tfs.URI()
	defer cleanupModelAndRemoveDir(m, tmpDir)

	sub := m.evLogger.Subscribe(events.RemoteIndexUpdated)
	defer sub.Unsubscribe()

	errPreventSync := errors.New("you aren't getting any of this")
	fc.RequestCalls(func(ctx context.Context, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
		return nil, errPreventSync
	})

	data := []byte("foo")
	num := 20
	for i := 0; i < num; i++ {
		fc.addFile(strconv.Itoa(i), 0644, protocol.FileInfoTypeFile, data)
	}
	fc.sendIndexUpdate()

	select {
	case <-sub.C():
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out before receiving index")
	}

	progress, queued, rest, err := m.NeedFolderFiles(fcfg.ID, 1, 100)
	must(t, err)
	if got := len(progress) + len(queued) + len(rest); got != num {
		t.Errorf("Got %v needed items, expected %v", got, num)
	}

	exp := 10
	for page := 1; page < 3; page++ {
		progress, queued, rest, err := m.NeedFolderFiles(fcfg.ID, page, exp)
		must(t, err)
		if got := len(progress) + len(queued) + len(rest); got != exp {
			t.Errorf("Got %v needed items on page %v, expected %v", got, page, exp)
		}
	}
}

// TestIgnoreDeleteUnignore checks that the deletion of an ignored file is not
// propagated upon un-ignoring.
// https://github.com/syncthing/syncthing/issues/6038
func TestIgnoreDeleteUnignore(t *testing.T) {
	w, fcfg, wCancel := tmpDefaultWrapper(t)
	defer wCancel()
	m := setupModel(t, w)
	fss := fcfg.Filesystem(nil)
	tmpDir := fss.URI()
	defer cleanupModelAndRemoveDir(m, tmpDir)

	folderIgnoresAlwaysReload(t, m, fcfg)
	m.ScanFolders()

	fc := addFakeConn(m, device1, fcfg.ID)
	fc.folder = "default"
	fc.mut.Lock()
	fc.mut.Unlock()

	file := "foobar"
	contents := []byte("test file contents\n")

	basicCheck := func(fs []protocol.FileInfo) {
		t.Helper()
		if len(fs) != 1 {
			t.Fatal("expected a single index entry, got", len(fs))
		} else if fs[0].Name != file {
			t.Fatalf("expected a index entry for %v, got one for %v", file, fs[0].Name)
		}
	}

	done := make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		basicCheck(fs)
		close(done)
		return nil
	})

	writeFile(t, fss, file, contents)
	m.ScanFolders()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out before index was received")
	case <-done:
	}

	done = make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		basicCheck(fs)
		f := fs[0]
		if !f.IsInvalid() {
			t.Errorf("Received non-invalid index update")
		}
		close(done)
		return nil
	})

	if err := m.SetIgnores("default", []string{"foobar"}); err != nil {
		panic(err)
	}

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index update")
	case <-done:
	}

	done = make(chan struct{})
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		basicCheck(fs)
		f := fs[0]
		if f.IsInvalid() {
			t.Errorf("Received invalid index update")
		}
		if !f.Version.Equal(protocol.Vector{}) && f.Deleted {
			t.Error("Received deleted index entry with non-empty version")
		}
		l.Infoln(f)
		close(done)
		return nil
	})

	if err := fss.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := m.SetIgnores("default", []string{}); err != nil {
		panic(err)
	}

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out before index was received")
	case <-done:
	}
}

// TestRequestLastFileProgress checks that the last pulled file (here only) is registered
// as in progress.
func TestRequestLastFileProgress(t *testing.T) {
	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	done := make(chan struct{})

	fc.RequestCalls(func(ctx context.Context, folder, name string, blockNo int, offset int64, size int, hash []byte, weakHash uint32, fromTemporary bool) ([]byte, error) {
		defer close(done)
		progress, queued, rest, err := m.NeedFolderFiles(folder, 1, 10)
		must(t, err)
		if len(queued)+len(rest) != 0 {
			t.Error(`There should not be any queued or "rest" items`)
		}
		if len(progress) != 1 {
			t.Error("Expected exactly one item in progress.")
		}
		return fc.fileData[name], nil
	})

	contents := []byte("test file contents\n")
	fc.addFile("testfile", 0644, protocol.FileInfoTypeFile, contents)
	fc.sendIndexUpdate()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out before file was requested")
	}
}

func TestRequestIndexSenderPause(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	indexChan := make(chan []protocol.FileInfo)
	fc.setIndexFn(func(ctx context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case indexChan <- fs:
		case <-done:
		case <-ctx.Done():
		}
		return nil
	})

	var seq int64 = 1
	files := []protocol.FileInfo{{Name: "foo", Size: 10, Version: protocol.Vector{}.Update(myID.Short()), Sequence: seq}}

	// Both devices connected, none paused
	localIndexUpdate(m, fcfg.ID, files)
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index")
	case <-indexChan:
	}

	// Remote paused

	cc := basicClusterConfig(device1, myID, fcfg.ID)
	cc.Folders[0].Paused = true
	m.ClusterConfig(device1, cc)

	seq++
	files[0].Sequence = seq
	files[0].Version = files[0].Version.Update(myID.Short())
	localIndexUpdate(m, fcfg.ID, files)

	// I don't see what to hook into to ensure an index update is not sent.
	dur := 50 * time.Millisecond
	if !testing.Short() {
		dur = 2 * time.Second
	}
	select {
	case <-time.After(dur):
	case <-indexChan:
		t.Error("Received index despite remote being paused")
	}

	// Remote unpaused

	cc.Folders[0].Paused = false
	m.ClusterConfig(device1, cc)
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index")
	case <-indexChan:
	}

	// Local paused and resume

	pauseFolder(t, m.cfg, fcfg.ID, true)
	pauseFolder(t, m.cfg, fcfg.ID, false)

	seq++
	files[0].Sequence = seq
	files[0].Version = files[0].Version.Update(myID.Short())
	localIndexUpdate(m, fcfg.ID, files)
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index")
	case <-indexChan:
	}

	// Local and remote paused, then first resume remote, then local

	cc.Folders[0].Paused = true
	m.ClusterConfig(device1, cc)

	pauseFolder(t, m.cfg, fcfg.ID, true)

	cc.Folders[0].Paused = false
	m.ClusterConfig(device1, cc)

	pauseFolder(t, m.cfg, fcfg.ID, false)

	seq++
	files[0].Sequence = seq
	files[0].Version = files[0].Version.Update(myID.Short())
	localIndexUpdate(m, fcfg.ID, files)
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index")
	case <-indexChan:
	}

	// Folder removed on remote

	cc = protocol.ClusterConfig{}
	m.ClusterConfig(device1, cc)

	seq++
	files[0].Sequence = seq
	files[0].Version = files[0].Version.Update(myID.Short())
	localIndexUpdate(m, fcfg.ID, files)

	select {
	case <-time.After(dur):
	case <-indexChan:
		t.Error("Received index despite remote not having the folder")
	}
}

func TestRequestIndexSenderClusterConfigBeforeStart(t *testing.T) {
	w, fcfg, wCancel := tmpDefaultWrapper(t)
	defer wCancel()
	tfs := fcfg.Filesystem(nil)
	dir1 := "foo"
	dir2 := "bar"

	// Initialise db with an entry and then stop everything again
	must(t, tfs.Mkdir(dir1, 0777))
	m := newModel(t, w, myID, "syncthing", "dev", nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())
	m.ServeBackground()
	m.ScanFolders()
	m.cancel()
	<-m.stopped

	// Add connection (sends incoming cluster config) before starting the new model
	m = &testModel{
		model:    NewModel(m.cfg, m.id, m.clientName, m.clientVersion, m.db, m.protectedFiles, m.evLogger).(*model),
		evCancel: m.evCancel,
		stopped:  make(chan struct{}),
	}
	defer cleanupModel(m)
	fc := addFakeConn(m, device1, fcfg.ID)
	done := make(chan struct{})
	defer close(done) // Must be the last thing to be deferred, thus first to run.
	indexChan := make(chan []protocol.FileInfo, 1)
	ccChan := make(chan protocol.ClusterConfig, 1)
	fc.setIndexFn(func(_ context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case indexChan <- fs:
		case <-done:
		}
		return nil
	})
	fc.ClusterConfigCalls(func(cc protocol.ClusterConfig) {
		select {
		case ccChan <- cc:
		case <-done:
		}
	})

	m.ServeBackground()

	timeout := time.After(5 * time.Second)

	// Check that cluster-config is resent after adding folders when starting model
	select {
	case <-timeout:
		t.Fatal("timed out before receiving cluster-config")
	case <-ccChan:
	}

	// Check that an index is sent for the newly added item
	must(t, tfs.Mkdir(dir2, 0777))
	m.ScanFolders()
	select {
	case <-timeout:
		t.Fatal("timed out before receiving index")
	case <-indexChan:
	}
}

func TestRequestReceiveEncrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping on short testing - scrypt is too slow")
	}

	w, fcfg, wCancel := tmpDefaultWrapper(t)
	defer wCancel()
	tfs := fcfg.Filesystem(nil)
	fcfg.Type = config.FolderTypeReceiveEncrypted
	setFolder(t, w, fcfg)

	encToken := protocol.PasswordToken(fcfg.ID, "pw")
	must(t, tfs.Mkdir(config.DefaultMarkerName, 0777))
	must(t, writeEncryptionToken(encToken, fcfg))

	m := setupModel(t, w)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	files := genFiles(2)
	files[1].LocalFlags = protocol.FlagLocalReceiveOnly
	m.fmut.RLock()
	fset := m.folderFiles[fcfg.ID]
	m.fmut.RUnlock()
	fset.Update(protocol.LocalDeviceID, files)

	indexChan := make(chan []protocol.FileInfo, 10)
	done := make(chan struct{})
	defer close(done)
	fc := newFakeConnection(device1, m)
	fc.folder = fcfg.ID
	fc.setIndexFn(func(_ context.Context, _ string, fs []protocol.FileInfo) error {
		select {
		case indexChan <- fs:
		case <-done:
		}
		return nil
	})
	m.AddConnection(fc, protocol.Hello{})
	m.ClusterConfig(device1, protocol.ClusterConfig{
		Folders: []protocol.Folder{
			{
				ID: "default",
				Devices: []protocol.Device{
					{
						ID:                      myID,
						EncryptionPasswordToken: encToken,
					},
					{ID: device1},
				},
			},
		},
	})

	select {
	case fs := <-indexChan:
		if len(fs) != 1 {
			t.Error("Expected index with one file, got", fs)
		}
		if got := fs[0].Name; got != files[0].Name {
			t.Errorf("Expected file %v, got %v", got, files[0].Name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index")
	}

	// Detects deletion, as we never really created the file on disk
	// Shouldn't send anything because receive-encrypted
	must(t, m.ScanFolder(fcfg.ID))
	// One real file to be sent
	name := "foo"
	data := make([]byte, 2000)
	rand.Read(data)
	fc.addFile(name, 0664, protocol.FileInfoTypeFile, data)
	fc.sendIndexUpdate()

	select {
	case fs := <-indexChan:
		if len(fs) != 1 {
			t.Error("Expected index with one file, got", fs)
		}
		if got := fs[0].Name; got != name {
			t.Errorf("Expected file %v, got %v", got, files[0].Name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out before receiving index")
	}

	// Simulate request from device that is untrusted too, i.e. with non-empty, but garbage hash
	_, err := m.Request(device1, fcfg.ID, name, 0, 1064, 0, []byte("garbage"), 0, false)
	must(t, err)

	changed, err := m.LocalChangedFolderFiles(fcfg.ID, 1, 10)
	must(t, err)
	if l := len(changed); l != 1 {
		t.Errorf("Expected one locally changed file, got %v", l)
	} else if changed[0].Name != files[0].Name {
		t.Errorf("Expected %v, got %v", files[0].Name, changed[0].Name)
	}
}

func TestRequestGlobalInvalidToValid(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	m, fc, fcfg, wcfgCancel := setupModelWithConnection(t)
	defer wcfgCancel()
	fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{DeviceID: device2})
	waiter, err := m.cfg.Modify(func(cfg *config.Configuration) {
		cfg.SetDevice(newDeviceConfiguration(cfg.Defaults.Device, device2, "device2"))
		fcfg.Devices = append(fcfg.Devices, config.FolderDeviceConfiguration{DeviceID: device2})
		cfg.SetFolder(fcfg)
	})
	must(t, err)
	waiter.Wait()
	addFakeConn(m, device2, fcfg.ID)
	tfs := fcfg.Filesystem(nil)
	defer cleanupModelAndRemoveDir(m, tfs.URI())

	indexChan := make(chan []protocol.FileInfo, 1)
	fc.setIndexFn(func(ctx context.Context, folder string, fs []protocol.FileInfo) error {
		select {
		case indexChan <- fs:
		case <-done:
		case <-ctx.Done():
		}
		return nil
	})

	name := "foo"

	// Setup device with valid file, do not send index yet
	contents := []byte("test file contents\n")
	fc.addFile(name, 0644, protocol.FileInfoTypeFile, contents)

	// Third device ignoring the same file
	fc.mut.Lock()
	file := fc.files[0]
	fc.mut.Unlock()
	file.SetIgnored()
	m.IndexUpdate(device2, fcfg.ID, []protocol.FileInfo{prepareFileInfoForIndex(file)})

	// Wait for the ignored file to be received and possible pulled
	timeout := time.After(10 * time.Second)
	globalUpdated := false
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out (globalUpdated == %v)", globalUpdated)
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if !globalUpdated {
			_, ok, err := m.CurrentGlobalFile(fcfg.ID, name)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				continue
			}
			globalUpdated = true
		}
		snap, err := m.DBSnapshot(fcfg.ID)
		if err != nil {
			t.Fatal(err)
		}
		need := snap.NeedSize(protocol.LocalDeviceID)
		snap.Release()
		if need.Files == 0 {
			break
		}
	}

	// Send the valid file
	fc.sendIndexUpdate()

	gotInvalid := false
	for {
		select {
		case <-timeout:
			t.Fatal("timed out before receiving index")
		case fs := <-indexChan:
			if len(fs) != 1 {
				t.Fatalf("Expected one file in index, got %v", len(fs))
			}
			if !fs[0].IsInvalid() {
				return
			}
			if gotInvalid {
				t.Fatal("Received two invalid index updates")
			}
			t.Log("got index with invalid file")
			gotInvalid = true
		}
	}
}
