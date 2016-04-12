// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//+build darwin

package versioner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/osutil"
)

func TestFindTrashFolder(t *testing.T) {
	file, err := osutil.ExpandTilde("~")
	if err != nil {
		t.Fatal(err)
	}

	// In home directory

	info, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}
	devID, err := deviceID(info)
	if err != nil {
		t.Fatal(err)
	}

	folder, err := getTrashFolder("~", devID)
	if err != nil {
		t.Fatal(err)
	}

	homeTrash := filepath.Join(file, ".Trash")
	if folder != homeTrash {
		t.Errorf("Unexpected trash folder, got %q, expected %q", folder, homeTrash)
	}

	// On root volume, maybe same as home directory

	/* Can't test this on a normal system without manually mounting a disk image or something.

	info, err = os.Lstat("/Volumes/Untitled")
	if err != nil {
		t.Fatal(err)
	}

	folder, err = getTrashFolder("/Volumes/Untitled", info)
	if err != nil {
		t.Fatal(err)
	}

	rootTrash := "/Volumes/Untitled/.Trashes/" + strconv.Itoa(os.Getuid())
	if folder != rootTrash {
		t.Errorf("Unexpected trash folder, got %q, expected %q", folder, rootTrash)
	}
	*/

	// /dev is a separate volume and doesn't have a trash folder

	info, err = os.Lstat("/dev")
	if err != nil {
		t.Fatal(err)
	}
	devID, err = deviceID(info)
	if err != nil {
		t.Fatal(err)
	}

	folder, err = getTrashFolder("/dev", devID)
	if err == nil {
		t.Error("Unexpected nil error when looking for trash under /dev:", folder)
	}
}
