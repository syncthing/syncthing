// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package model

import (
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/sync"
)

// Test hiding dot files folder configuration
func TestHideDotFiles(t *testing.T) {
	tmpDir := createTmpDir()
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		filename     string
		hideDotFiles bool
		expected     bool // expected hidden state
	}{
		{".temp_name", true, true},
		{".another_temp_name", false, false},
		{"temp_name", true, false},
		{"another_temp_name", false, false},
	}

	for _, test := range tests {
		s := sharedPullerState{
			fs:           fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir),
			tempName:     test.filename,
			realName:     test.filename,
			mut:          sync.NewRWMutex(),
			hideDotFiles: test.hideDotFiles,
		}
		_, err := s.tempFile()
		if err != nil {
			t.Fatal(err)
		}
		s.finalClose()

		// Verify the file is in the expected hidden state
		// after the fineClose
		h := isHidden(path.Join(tmpDir, s.tempName))
		if h != test.expected {
			t.Fatalf("%s w/ hideDotFiles: %t expected hidden: %t, received: %t",
				test.filename, test.hideDotFiles, test.expected, h)
		}
	}
}

func isHidden(filepath string) bool {
	p, err := syscall.UTF16PtrFromString(filepath)
	if err != nil {
		return false
	}

	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return false
	}

	return attrs&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
