// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

func TestFileInfo(t *testing.T) {
	fs, dir := setup(t)
	path := filepath.Join(dir, "file")

	fd, err := fs.Create("file")
	if err != nil {
		t.Error(err)
	}
	defer fd.Close()

	ofi, err := os.Stat(path)
	if err != nil {
		t.Error(err)
	}

	bfi, err := fd.Stat()
	if err != nil {
		t.Error(err)
	}

	sfi, err := fs.Stat(fd.Name())
	if err != nil {
		t.Error(err)
	}

	if bfi.Name() != ofi.Name() {
		t.Errorf("Name(): got %v, want %v", bfi.Name(), ofi.Name())
	}
	if bfi.Name() != sfi.Name() {
		t.Errorf("Name(): got %v, want %v", bfi.Name(), sfi.Name())
	}
	if bfi.Size() != ofi.Size() {
		t.Errorf("Size(): got %v, want %v", bfi.Size(), ofi.Size())
	}
	if bfi.Size() != sfi.Size() {
		t.Errorf("Size(): got %v, want %v", bfi.Size(), sfi.Size())
	}
	if bfi.ModTime() != ofi.ModTime() {
		t.Errorf("ModTime(): got %v, want %v", bfi.ModTime(), ofi.ModTime())
	}
	if bfi.ModTime() != sfi.ModTime() {
		t.Errorf("ModTime(): got %v, want %v", bfi.ModTime(), sfi.ModTime())
	}
	omode := uint32(ofi.Mode())
	bmode := uint32(bfi.Mode())
	smode := uint32(sfi.Mode())
	if build.IsWindows {
		omode &^= uint32(0o022)
	}
	if omode != bmode {
		t.Errorf("Mode(): got 0o%o, want 0o%o", omode, bmode)
	}
	if omode != smode {
		t.Errorf("Mode(): got 0o%o, want 0o%o", omode, smode)
	}
	if bmode != smode {
		t.Errorf("Mode(): got 0o%o, want 0o%o", bmode, smode)
	}

	if bfi.InodeChangeTime() != sfi.InodeChangeTime() {
		t.Errorf("InodeChangeTime(): got %v, want %v", bfi.InodeChangeTime(), sfi.InodeChangeTime())
	}

	if bfi.InodeChangeTime().IsZero() {
		t.Log("InodeChangeTime() is 0 on" + runtime.GOOS)
		return
	}
	if sfi.InodeChangeTime().IsZero() {
		t.Log("InodeChangeTime() is 0 on" + runtime.GOOS)
		return
	}

	if !withinASecond(bfi.InodeChangeTime(), bfi.ModTime()) {
		t.Errorf("InodeChangeTime(): %v != %v", bfi.InodeChangeTime(), bfi.ModTime())
	}
	if !withinASecond(sfi.InodeChangeTime(), sfi.ModTime()) {
		t.Errorf("InodeChangeTime(): %v != %v", sfi.InodeChangeTime(), sfi.ModTime())
	}

	time.Sleep(time.Duration(2) * time.Second)

	err = appendToFile(path, "some text")
	if err != nil {
		t.Error(err)
	}

	bfi, err = fd.Stat()
	if err != nil {
		t.Error(err)
	}

	sfi, err = fs.Stat(fd.Name())
	if err != nil {
		t.Error(err)
	}

	if !withinASecond(bfi.InodeChangeTime(), bfi.ModTime()) {
		t.Errorf("InodeChangeTime(): %v != %v", bfi.InodeChangeTime(), bfi.ModTime())
	}
	if !withinASecond(sfi.InodeChangeTime(), sfi.ModTime()) {
		t.Errorf("InodeChangeTime(): %v != %v", sfi.InodeChangeTime(), sfi.ModTime())
	}

	time.Sleep(time.Duration(2) * time.Second)

	err = os.Chmod(path, 0o444)
	if err != nil {
		t.Error(err)
	}
	err = os.Chmod(path, 0o666)
	if err != nil {
		t.Error(err)
	}

	bfi, err = fd.Stat()
	if err != nil {
		t.Error(err)
	}

	sfi, err = fs.Stat(fd.Name())
	if err != nil {
		t.Error(err)
	}

	diff := bfi.InodeChangeTime().Sub(bfi.ModTime()).Seconds()
	if diff < 1 {
		t.Errorf("InodeChangeTime(): %v - %v = %v", bfi.InodeChangeTime(), bfi.ModTime(), diff)
	}
	diff = sfi.InodeChangeTime().Sub(sfi.ModTime()).Seconds()
	if diff < 1 {
		t.Errorf("InodeChangeTime(): %v - %v = %v", sfi.InodeChangeTime(), sfi.ModTime(), diff)
	}
}

func withinASecond(a, b time.Time) bool {
	return a.Sub(b).Abs() < 1*time.Second
}

func appendToFile(path, data string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(data)
	return err
}
