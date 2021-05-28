// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

func TestRecvOnlyRevertDeletes(t *testing.T) {
	// Make sure that we delete extraneous files and directories when we hit
	// Revert.

	// Get us a model up and running

	m, f, wcfgCancel := setupROFolder(t)
	defer wcfgCancel()
	ffs := f.Filesystem()
	defer cleanupModel(m)

	// Create some test data

	for _, dir := range []string{".stfolder", "ignDir", "unknownDir"} {
		must(t, ffs.MkdirAll(dir, 0755))
	}
	must(t, writeFile(ffs, "ignDir/ignFile", []byte("hello\n"), 0644))
	must(t, writeFile(ffs, "unknownDir/unknownFile", []byte("hello\n"), 0644))
	must(t, writeFile(ffs, ".stignore", []byte("ignDir\n"), 0644))

	knownFiles := setupKnownFiles(t, ffs, []byte("hello\n"))

	// Send and index update for the known stuff

	must(t, m.Index(device1, "ro", knownFiles))
	f.updateLocalsFromScanning(knownFiles)

	size := globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}

	// Scan, should discover the other stuff in the folder

	must(t, m.ScanFolder("ro"))

	// We should now have two files and two directories, with global state unchanged.

	size = globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 2 files and 2 directories: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 2 || size.Directories != 2 {
		t.Fatalf("Local: expected 2 files and 2 directories: %+v", size)
	}
	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories == 0 {
		t.Fatalf("ROChanged: expected something: %+v", size)
	}

	// Revert should delete the unknown stuff

	m.Revert("ro")

	// These should still exist
	for _, p := range []string{"knownDir/knownFile", "ignDir/ignFile"} {
		if _, err := ffs.Stat(p); err != nil {
			t.Error("Unexpected error:", err)
		}
	}

	// These should have been removed
	for _, p := range []string{"unknownDir", "unknownDir/unknownFile"} {
		if _, err := ffs.Stat(p); !fs.IsNotExist(err) {
			t.Error("Unexpected existing thing:", p)
		}
	}

	// We should now have one file and directory again.

	size = globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 files and 1 directories: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 files and 1 directories: %+v", size)
	}
}

func TestRecvOnlyRevertNeeds(t *testing.T) {
	// Make sure that a new file gets picked up and considered latest, then
	// gets considered old when we hit Revert.

	// Get us a model up and running

	m, f, wcfgCancel := setupROFolder(t)
	defer wcfgCancel()
	ffs := f.Filesystem()
	defer cleanupModel(m)

	// Create some test data

	must(t, ffs.MkdirAll(".stfolder", 0755))
	oldData := []byte("hello\n")
	knownFiles := setupKnownFiles(t, ffs, oldData)

	// Send and index update for the known stuff

	must(t, m.Index(device1, "ro", knownFiles))
	f.updateLocalsFromScanning(knownFiles)

	// Scan the folder.

	must(t, m.ScanFolder("ro"))

	// Everything should be in sync.

	size := globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 file and 1 directory: %+v", size)
	}
	size = needSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("ROChanged: expected nothing: %+v", size)
	}

	// Update the file.

	newData := []byte("totally different data\n")
	must(t, writeFile(ffs, "knownDir/knownFile", newData, 0644))

	// Rescan.

	must(t, m.ScanFolder("ro"))

	// We now have a newer file than the rest of the cluster. Global state should reflect this.

	size = globalSize(t, m, "ro")
	const sizeOfDir = 128
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(oldData)) {
		t.Fatalf("Global: expected no change due to the new file: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(newData)) {
		t.Fatalf("Local: expected the new file to be reflected: %+v", size)
	}
	size = needSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories == 0 {
		t.Fatalf("ROChanged: expected something: %+v", size)
	}

	// We hit the Revert button. The file that was new should become old.

	m.Revert("ro")

	size = globalSize(t, m, "ro")
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(oldData)) {
		t.Fatalf("Global: expected the global size to revert: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Bytes != sizeOfDir+int64(len(newData)) {
		t.Fatalf("Local: expected the local size to remain: %+v", size)
	}
	size = needSize(t, m, "ro")
	if size.Files != 1 || size.Bytes != int64(len(oldData)) {
		t.Fatalf("Local: expected to need the old file data: %+v", size)
	}
}

func TestRecvOnlyUndoChanges(t *testing.T) {
	// Get us a model up and running

	m, f, wcfgCancel := setupROFolder(t)
	defer wcfgCancel()
	ffs := f.Filesystem()
	defer cleanupModel(m)

	// Create some test data

	must(t, ffs.MkdirAll(".stfolder", 0755))
	oldData := []byte("hello\n")
	knownFiles := setupKnownFiles(t, ffs, oldData)

	// Send an index update for the known stuff

	must(t, m.Index(device1, "ro", knownFiles))
	f.updateLocalsFromScanning(knownFiles)

	// Scan the folder.

	must(t, m.ScanFolder("ro"))

	// Everything should be in sync.

	size := globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 file and 1 directory: %+v", size)
	}
	size = needSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("ROChanged: expected nothing: %+v", size)
	}

	// Create a file and modify another

	const file = "foo"
	must(t, writeFile(ffs, file, []byte("hello\n"), 0644))
	must(t, writeFile(ffs, "knownDir/knownFile", []byte("bye\n"), 0644))

	must(t, m.ScanFolder("ro"))

	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files != 2 {
		t.Fatalf("Receive only: expected 2 files: %+v", size)
	}

	// Remove the file again and undo the modification

	must(t, ffs.Remove(file))
	must(t, writeFile(ffs, "knownDir/knownFile", oldData, 0644))
	must(t, ffs.Chtimes("knownDir/knownFile", knownFiles[1].ModTime(), knownFiles[1].ModTime()))

	must(t, m.ScanFolder("ro"))

	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories+size.Deleted != 0 {
		t.Fatalf("Receive only: expected all zero: %+v", size)
	}
}

func TestRecvOnlyDeletedRemoteDrop(t *testing.T) {
	// Get us a model up and running

	m, f, wcfgCancel := setupROFolder(t)
	defer wcfgCancel()
	ffs := f.Filesystem()
	defer cleanupModel(m)

	// Create some test data

	must(t, ffs.MkdirAll(".stfolder", 0755))
	oldData := []byte("hello\n")
	knownFiles := setupKnownFiles(t, ffs, oldData)

	// Send an index update for the known stuff

	must(t, m.Index(device1, "ro", knownFiles))
	f.updateLocalsFromScanning(knownFiles)

	// Scan the folder.

	must(t, m.ScanFolder("ro"))

	// Everything should be in sync.

	size := globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 file and 1 directory: %+v", size)
	}
	size = needSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("ROChanged: expected nothing: %+v", size)
	}

	// Delete our file

	must(t, ffs.Remove(knownFiles[1].Name))

	must(t, m.ScanFolder("ro"))

	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Deleted != 1 {
		t.Fatalf("Receive only: expected 1 deleted: %+v", size)
	}

	// Drop the remote

	f.fset.Drop(device1)
	must(t, m.ScanFolder("ro"))

	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Deleted != 0 {
		t.Fatalf("Receive only: expected no deleted: %+v", size)
	}
}

func TestRecvOnlyRemoteUndoChanges(t *testing.T) {
	// Get us a model up and running

	m, f, wcfgCancel := setupROFolder(t)
	defer wcfgCancel()
	ffs := f.Filesystem()
	defer cleanupModel(m)

	// Create some test data

	must(t, ffs.MkdirAll(".stfolder", 0755))
	oldData := []byte("hello\n")
	knownFiles := setupKnownFiles(t, ffs, oldData)

	// Send an index update for the known stuff

	must(t, m.Index(device1, "ro", knownFiles))
	f.updateLocalsFromScanning(knownFiles)

	// Scan the folder.

	must(t, m.ScanFolder("ro"))

	// Everything should be in sync.

	size := globalSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Global: expected 1 file and 1 directory: %+v", size)
	}
	size = localSize(t, m, "ro")
	if size.Files != 1 || size.Directories != 1 {
		t.Fatalf("Local: expected 1 file and 1 directory: %+v", size)
	}
	size = needSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("Need: expected nothing: %+v", size)
	}
	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories > 0 {
		t.Fatalf("ROChanged: expected nothing: %+v", size)
	}

	// Create a file and modify another

	const file = "foo"
	knownFile := filepath.Join("knownDir", "knownFile")
	must(t, writeFile(ffs, file, []byte("hello\n"), 0644))
	must(t, writeFile(ffs, knownFile, []byte("bye\n"), 0644))

	must(t, m.ScanFolder("ro"))

	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files != 2 {
		t.Fatalf("Receive only: expected 2 files: %+v", size)
	}

	// Do the same changes on the remote

	files := make([]protocol.FileInfo, 0, 2)
	snap := fsetSnapshot(t, f.fset)
	snap.WithHave(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
		if n := fi.FileName(); n != file && n != knownFile {
			return true
		}
		f := fi.(protocol.FileInfo)
		f.LocalFlags = 0
		f.Version = protocol.Vector{}.Update(device1.Short())
		files = append(files, f)
		return true
	})
	snap.Release()
	must(t, m.IndexUpdate(device1, "ro", files))

	// Ensure the pull to resolve conflicts (content identical) happened
	must(t, f.doInSync(func() error {
		f.pull()
		return nil
	}))

	size = receiveOnlyChangedSize(t, m, "ro")
	if size.Files+size.Directories+size.Deleted != 0 {
		t.Fatalf("Receive only: expected all zero: %+v", size)
	}
}

func TestRecvOnlyRevertOwnID(t *testing.T) {
	// If the folder was receive-only in the past, the global item might have
	// only our id in the version vector and be valid. There was a bug based on
	// the incorrect assumption that this can never happen.

	// Get us a model up and running

	m, f, wcfgCancel := setupROFolder(t)
	defer wcfgCancel()
	ffs := f.Filesystem()
	defer cleanupModel(m)

	// Create some test data

	must(t, ffs.MkdirAll(".stfolder", 0755))
	data := []byte("hello\n")
	name := "foo"
	must(t, writeFile(ffs, name, data, 0644))

	// Make sure the file is scanned and locally changed
	must(t, m.ScanFolder("ro"))
	fi, ok := m.testCurrentFolderFile(f.ID, name)
	if !ok {
		t.Fatal("File missing")
	} else if !fi.IsReceiveOnlyChanged() {
		t.Fatal("File should be receiveonly changed")
	}
	fi.LocalFlags = 0
	v := fi.Version.Counters[0].Value
	fi.Version.Counters[0].Value = uint64(time.Unix(int64(v), 0).Add(-10 * time.Second).Unix())

	// Monitor the outcome
	sub := f.evLogger.Subscribe(events.LocalIndexUpdated)
	defer sub.Unsubscribe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sub.C():
				if file, _ := m.testCurrentFolderFile(f.ID, name); file.Deleted {
					t.Error("local file was deleted")
					cancel()
				} else if file.IsEquivalent(fi, f.modTimeWindow) {
					cancel() // That's what we are waiting for
				}
			}
		}
	}()

	// Receive an index update with an older version, but valid and then revert
	must(t, m.Index(device1, f.ID, []protocol.FileInfo{fi}))
	f.Revert()

	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}

func setupKnownFiles(t *testing.T, ffs fs.Filesystem, data []byte) []protocol.FileInfo {
	t.Helper()

	must(t, ffs.MkdirAll("knownDir", 0755))
	must(t, writeFile(ffs, "knownDir/knownFile", data, 0644))

	t0 := time.Now().Add(-1 * time.Minute)
	must(t, ffs.Chtimes("knownDir/knownFile", t0, t0))

	fi, err := ffs.Stat("knownDir/knownFile")
	if err != nil {
		t.Fatal(err)
	}
	blocks, _ := scanner.Blocks(context.TODO(), bytes.NewReader(data), protocol.BlockSize(int64(len(data))), int64(len(data)), nil, true)
	knownFiles := []protocol.FileInfo{
		{
			Name:        "knownDir",
			Type:        protocol.FileInfoTypeDirectory,
			Permissions: 0755,
			Version:     protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 42}}},
			Sequence:    42,
		},
		{
			Name:        "knownDir/knownFile",
			Type:        protocol.FileInfoTypeFile,
			Permissions: 0644,
			Size:        fi.Size(),
			ModifiedS:   fi.ModTime().Unix(),
			ModifiedNs:  int(fi.ModTime().UnixNano() % 1e9),
			Version:     protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 42}}},
			Sequence:    42,
			Blocks:      blocks,
		},
	}

	return knownFiles
}

func setupROFolder(t *testing.T) (*testModel, *receiveOnlyFolder, context.CancelFunc) {
	t.Helper()

	w, cancel := createTmpWrapper(defaultCfg)
	cfg := w.RawCopy()
	fcfg := testFolderConfigFake()
	fcfg.ID = "ro"
	fcfg.Label = "ro"
	fcfg.Type = config.FolderTypeReceiveOnly
	cfg.Folders = []config.FolderConfiguration{fcfg}
	replace(t, w, cfg)

	m := newModel(t, w, myID, "syncthing", "dev", nil)
	m.ServeBackground()
	<-m.started
	must(t, m.ScanFolder("ro"))

	m.fmut.RLock()
	defer m.fmut.RUnlock()
	f := m.folderRunners["ro"].(*receiveOnlyFolder)

	return m, f, cancel
}

func writeFile(fs fs.Filesystem, filename string, data []byte, perm fs.FileMode) error {
	fd, err := fs.Create(filename)
	if err != nil {
		return err
	}
	_, err = fd.Write(data)
	if err != nil {
		return err
	}
	if err := fd.Close(); err != nil {
		return err
	}
	return fs.Chmod(filename, perm)
}
