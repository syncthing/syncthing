// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

const maxDifference = time.Duration(100) * time.Millisecond

// See https://github.com/syncthing/syncthing/pull/10172#discussion_r2143661512
func TestFileInfo(t *testing.T) {
	t.Parallel()

	fs, dir := setup(t)
	basename := "fileinfo.txt"
	path := filepath.Join(dir, basename)

	// Sleep until the next 0.1s mark, to attempt to create the file with
	// fractional seconds.
	const target = 100_000_000 // 0.1s in ns
	remainder := time.Now().Nanosecond() % target
	if remainder != 0 {
		time.Sleep(time.Duration(target - remainder))
	}

	err := WriteFile(fs, basename, []byte("some text"), 0o644)
	if err != nil {
		t.Error(err)
	}

	// collect baseline timestamps via Lstat and InodeChangeTime
	osfi, err := os.Lstat(path)
	if err != nil {
		t.Error(err)
	}

	fi, err := fs.Lstat(basename)
	if err != nil {
		t.Error(err)
	}

	if fi.Name() != osfi.Name() {
		t.Errorf("Name(): got %v, want %v", fi.Name(), osfi.Name())
	}
	if fi.Size() != osfi.Size() {
		t.Errorf("Size(): got %v, want %v", fi.Size(), osfi.Size())
	}
	if fi.ModTime() != osfi.ModTime() {
		t.Errorf("ModTime(): got %v, want %v", fi.ModTime(), osfi.ModTime())
	}
	mode := uint32(fi.Mode())
	osmode := uint32(osfi.Mode())
	if build.IsWindows {
		osmode &^= uint32(0o022)
	}
	if mode != osmode {
		t.Errorf("Mode(): got 0o%o, want 0o%o", mode, osmode)
	}
	if fi.InodeChangeTime().IsZero() {
		t.Log("InodeChangeTime() is 0 on" + runtime.GOOS)
		return
	}

	diff := fi.InodeChangeTime().Sub(fi.ModTime())
	if diff.Abs() > maxDifference {
		t.Errorf("InodeChangeTime(): diff > %v: %v, %v", maxDifference, fi.InodeChangeTime(), fi.ModTime())
	}

	if fi.ModTime().Nanosecond() == 0 {
		// if the timestamps returned are second precision only (no nanoseconds part),
		// skip the rest of the test as we're running on a bad fs...
		return
	}

	time.Sleep(maxDifference * 2)

	err = updateFile(path, "some text")
	if err != nil {
		t.Error(err)
	}

	fi2, err := fs.Lstat(basename)
	if err != nil {
		t.Error(err)
	}

	// stat and compare -- modtime should have changed from the baseline, inode change time should not (but often does)
	diff = fi2.ModTime().Sub(fi.ModTime())
	if diff < maxDifference {
		t.Errorf("ModTime(): diff = %v: %v %v", diff, fi2.ModTime(), fi.ModTime())
	}

	diff = fi2.InodeChangeTime().Sub(fi.InodeChangeTime())
	// On a least Darwin, Linux, and Windows, the inode change time is updated when a file is appended to.
	// But let's not report an error, as this may not be the case on all filesystems.
	if diff == 0 {
		t.Logf("InodeChangeTime(): diff = %v: %v %v", diff, fi2.InodeChangeTime(), fi.InodeChangeTime())
	}

	// required since ModTime and InodeChangeTime are often the same.
	time.Sleep(maxDifference * 2)

	// Reset the user write bit, as that's the only bit that Windows will update.
	err = os.Chmod(path, 0o400)
	if err != nil {
		t.Error(err)
	}

	// stat and compare -- modtime should not have changed since last time, inode change time should have changed
	fi3, err := fs.Lstat(basename)
	if err != nil {
		t.Error(err)
	}

	diff = fi3.ModTime().Sub(fi2.ModTime())
	if diff != 0 {
		t.Errorf("ModTime(): diff = %v: %v %v", diff, fi3.ModTime(), fi2.ModTime())
	}

	diff = fi3.InodeChangeTime().Sub(fi2.InodeChangeTime())
	if diff == 0 {
		t.Errorf("InodeChangeTime(): diff = %v: %v %v", diff, fi3.InodeChangeTime(), fi2.InodeChangeTime())
	}
}

func updateFile(path, data string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(data)
	return err
}

func BenchmarkFileInfo(b *testing.B) {
	paths := filepath.SplitList(os.Getenv("STFSTESTPATH"))
	for _, path := range paths {
		b.Logf("Walking %v", path)
		fs := NewWalkFilesystem(newBasicFilesystem(path))

		var results = map[string]int{}
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			start := time.Now()
			err := fs.Walk("/", func(path string, _ FileInfo, err error) error {
				results[errorMessage(err)]++
				return nil
			})
			elapsed := time.Since(start)
			total := 0
			b.Log("Scanned Result")
			b.Log("------- ------")

			for k, v := range results {
				b.Logf("%7d %v", v, k)
				total += v
			}
			b.Log("-------")
			avg := elapsed.Seconds() / float64(total)
			b.Logf("%7d total files scanned in %v (%g per file)", total, elapsed, avg)
			if err != nil {
				b.Fatal(err)
			}
		})
	}
}

func errorMessage(err error) string {
	if err == nil {
		return "No error"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno.Error()
	}
	msg := err.Error()
	if i := lastColonSpace(msg); i >= 0 {
		return msg[i+2:]
	}

	return msg
}

func lastColonSpace(s string) int {
	for i := len(s) - 2; i >= 0; i-- {
		if s[i] == ':' && s[i+1] == ' ' {
			return i
		}
	}

	return -1
}
