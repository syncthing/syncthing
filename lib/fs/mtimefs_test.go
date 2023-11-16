// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

func TestMtimeFS(t *testing.T) {
	td := t.TempDir()
	os.Mkdir(filepath.Join(td, "testdata"), 0o755)
	os.WriteFile(filepath.Join(td, "testdata", "exists0"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(td, "testdata", "exists1"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(td, "testdata", "exists2"), []byte("hello"), 0o644)

	// a random time with nanosecond precision
	testTime := time.Unix(1234567890, 123456789)

	mtimefs := newMtimeFS(td, make(mapStore))

	// Do one Chtimes call that will go through to the normal filesystem
	mtimefs.chtimes = os.Chtimes
	if err := mtimefs.Chtimes("testdata/exists0", testTime, testTime); err != nil {
		t.Error("Should not have failed:", err)
	}

	// Do one call that gets an error back from the underlying Chtimes
	mtimefs.chtimes = failChtimes
	if err := mtimefs.Chtimes("testdata/exists1", testTime, testTime); err != nil {
		t.Error("Should not have failed:", err)
	}

	// Do one call that gets struck by an exceptionally evil Chtimes
	mtimefs.chtimes = evilChtimes
	if err := mtimefs.Chtimes("testdata/exists2", testTime, testTime); err != nil {
		t.Error("Should not have failed:", err)
	}

	// All of the calls were successful, so an Lstat on them should return
	// the test timestamp.

	for _, file := range []string{"testdata/exists0", "testdata/exists1", "testdata/exists2"} {
		if info, err := mtimefs.Lstat(file); err != nil {
			t.Error("Lstat shouldn't fail:", err)
		} else if !info.ModTime().Equal(testTime) {
			t.Errorf("Time mismatch; %v != expected %v", info.ModTime(), testTime)
		}
	}

	// The two last files should certainly not have the correct timestamp
	// when looking directly on disk though.

	for _, file := range []string{"testdata/exists1", "testdata/exists2"} {
		if info, err := os.Lstat(filepath.Join(td, file)); err != nil {
			t.Error("Lstat shouldn't fail:", err)
		} else if info.ModTime().Equal(testTime) {
			t.Errorf("Unexpected time match; %v == %v", info.ModTime(), testTime)
		}
	}

	// Changing the timestamp on disk should be reflected in a new Lstat
	// call. Choose a time that is likely to be able to be on all reasonable
	// filesystems.

	testTime = time.Now().Add(5 * time.Hour).Truncate(time.Minute)
	os.Chtimes(filepath.Join(td, "testdata/exists0"), testTime, testTime)
	if info, err := mtimefs.Lstat("testdata/exists0"); err != nil {
		t.Error("Lstat shouldn't fail:", err)
	} else if !info.ModTime().Equal(testTime) {
		t.Errorf("Time mismatch; %v != expected %v", info.ModTime(), testTime)
	}
}

func TestHackFATDST(t *testing.T) {
	td := t.TempDir()

	testFile := filepath.Join(td, "file")
	os.WriteFile(testFile, []byte("hello"), 0o644)

	// A timestamp that looks like it belongs on a FAT filesystem;
	// two-second precision only.
	ts := time.Now().Truncate(2 * time.Second)
	if err := os.Chtimes(testFile, ts, ts); err != nil {
		t.Fatal(err)
	}

	mtimefs := newMtimeFS(td, make(mapStore))
	mtimefs.hackFATDST = true

	// Check the file; it should have its original timestamp.
	if info, err := mtimefs.Lstat("file"); err != nil {
		t.Error("Lstat shouldn't fail:", err)
	} else if !info.ModTime().Equal(ts) {
		t.Errorf("Unexpected time mismatch; %v != %v", info.ModTime(), ts)
	}

	// Change the timestamp by precisely one hour, simulating a DST change.
	dst := ts.Add(time.Hour)
	if err := os.Chtimes(testFile, dst, dst); err != nil {
		t.Fatal(err)
	}

	// Check the file; it should still have its original timestamp.
	if info, err := mtimefs.Lstat("file"); err != nil {
		t.Error("Lstat shouldn't fail:", err)
	} else if !info.ModTime().Equal(ts) {
		t.Errorf("Unexpected time mismatch; %v != %v", info.ModTime(), ts)
	}

	// Instead, change the timestamp by one hour plus a second.
	other := ts.Add(time.Hour).Add(time.Second)
	if err := os.Chtimes(testFile, other, other); err != nil {
		t.Fatal(err)
	}

	// Check the file; the new timestamp should shine through.
	if info, err := mtimefs.Lstat("file"); err != nil {
		t.Error("Lstat shouldn't fail:", err)
	} else if !info.ModTime().Equal(other) {
		t.Errorf("Unexpected time mismatch; %v != %v", info.ModTime(), other)
	}
}

func TestMtimeFSWalk(t *testing.T) {
	dir := t.TempDir()

	mtimefs, walkFs := newMtimeFSWithWalk(dir, make(mapStore))
	underlying := mtimefs.Filesystem
	mtimefs.chtimes = failChtimes

	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStat, err := mtimefs.Lstat("file")
	if err != nil {
		t.Fatal(err)
	}

	newTime := time.Now().Add(-2 * time.Hour)

	if err := mtimefs.Chtimes("file", newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if newStat, err := mtimefs.Lstat("file"); err != nil {
		t.Fatal(err)
	} else if !newStat.ModTime().Equal(newTime) {
		t.Errorf("expected time %v, lstat time %v", newTime, newStat.ModTime())
	}

	if underlyingStat, err := underlying.Lstat("file"); err != nil {
		t.Fatal(err)
	} else if !underlyingStat.ModTime().Equal(oldStat.ModTime()) {
		t.Errorf("expected time %v, lstat time %v", oldStat.ModTime(), underlyingStat.ModTime())
	}

	found := false
	_ = walkFs.Walk("", func(path string, info FileInfo, err error) error {
		if path == "file" {
			found = true
			if !info.ModTime().Equal(newTime) {
				t.Errorf("expected time %v, lstat time %v", newTime, info.ModTime())
			}
		}
		return nil
	})

	if !found {
		t.Error("did not find")
	}
}

func TestMtimeFSOpen(t *testing.T) {
	dir := t.TempDir()

	mtimefs := newMtimeFS(dir, make(mapStore))
	underlying := mtimefs.Filesystem
	mtimefs.chtimes = failChtimes

	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStat, err := mtimefs.Lstat("file")
	if err != nil {
		t.Fatal(err)
	}

	newTime := time.Now().Add(-2 * time.Hour)

	if err := mtimefs.Chtimes("file", newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if newStat, err := mtimefs.Lstat("file"); err != nil {
		t.Fatal(err)
	} else if !newStat.ModTime().Equal(newTime) {
		t.Errorf("expected time %v, lstat time %v", newTime, newStat.ModTime())
	}

	if underlyingStat, err := underlying.Lstat("file"); err != nil {
		t.Fatal(err)
	} else if !underlyingStat.ModTime().Equal(oldStat.ModTime()) {
		t.Errorf("expected time %v, lstat time %v", oldStat.ModTime(), underlyingStat.ModTime())
	}

	fd, err := mtimefs.Open("file")
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	info, err := fd.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(newTime) {
		t.Errorf("expected time %v, lstat time %v", newTime, info.ModTime())
	}
}

func TestMtimeFSInsensitive(t *testing.T) {
	if build.IsDarwin || build.IsWindows {
		// blatantly assume file systems here are case insensitive. Might be
		// a spurious failure on oddly configured systems.
	} else {
		t.Skip("need case insensitive FS")
	}

	theTest := func(t *testing.T, fs *mtimeFS, shouldSucceed bool) {
		fs.RemoveAll("testdata")
		defer fs.RemoveAll("testdata")
		fs.Mkdir("testdata", 0o755)
		WriteFile(fs, "testdata/FiLe", []byte("hello"), 0o644)

		// a random time with nanosecond precision
		testTime := time.Unix(1234567890, 123456789)

		// Do one call that gets struck by an exceptionally evil Chtimes, with a
		// different case from what is on disk.
		fs.chtimes = evilChtimes
		if err := fs.Chtimes("testdata/fIlE", testTime, testTime); err != nil {
			t.Error("Should not have failed:", err)
		}

		// Check that we get back the mtime we set, if we were supposed to succeed.
		info, err := fs.Lstat("testdata/FILE")
		if err != nil {
			t.Error("Lstat shouldn't fail:", err)
		} else if info.ModTime().Equal(testTime) != shouldSucceed {
			t.Errorf("Time mismatch; got %v, comparison %v, expected equal=%v", info.ModTime(), testTime, shouldSucceed)
		}
	}

	// The test should fail with a case sensitive mtimefs
	t.Run("with case sensitive mtimefs", func(t *testing.T) {
		theTest(t, newMtimeFS(t.TempDir(), make(mapStore)), false)
	})

	// And succeed with a case insensitive one.
	t.Run("with case insensitive mtimefs", func(t *testing.T) {
		theTest(t, newMtimeFS(t.TempDir(), make(mapStore), WithCaseInsensitivity(true)), true)
	})
}

// The mapStore is a simple database

type mapStore map[string][]byte

func (s mapStore) PutBytes(key string, data []byte) error {
	s[key] = data
	return nil
}

func (s mapStore) Bytes(key string) (data []byte, ok bool, err error) {
	data, ok = s[key]
	return
}

func (s mapStore) Delete(key string) error {
	delete(s, key)
	return nil
}

// failChtimes does nothing, and fails
func failChtimes(_ string, _, _ time.Time) error {
	return errors.New("no")
}

// evilChtimes will set an mtime that's 300 days in the future of what was
// asked for, and truncate the time to the closest hour.
func evilChtimes(name string, mtime, atime time.Time) error {
	return os.Chtimes(name, mtime.Add(300*time.Hour).Truncate(time.Hour), atime.Add(300*time.Hour).Truncate(time.Hour))
}

func newMtimeFS(path string, db database, options ...MtimeFSOption) *mtimeFS {
	mtimefs, _ := newMtimeFSWithWalk(path, db, options...)
	return mtimefs
}

func newMtimeFSWithWalk(path string, db database, options ...MtimeFSOption) (*mtimeFS, *walkFilesystem) {
	fs := NewFilesystem(FilesystemTypeBasic, path, NewMtimeOption(db, options...))
	wfs, _ := unwrapFilesystem(fs, filesystemWrapperTypeWalk)
	mfs, _ := unwrapFilesystem(fs, filesystemWrapperTypeMtime)
	return mfs.(*mtimeFS), wfs.(*walkFilesystem)
}
