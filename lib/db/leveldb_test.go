// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"testing"
)

func TestDeviceKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")
	dev := []byte("device67890123456789012345678901")
	name := []byte("name")

	key := deviceKey(fld, dev, name)

	fld2 := deviceKeyFolder(key)
	if bytes.Compare(fld2, fld) != 0 {
		t.Errorf("wrong folder %q != %q", fld2, fld)
	}
	dev2 := deviceKeyDevice(key)
	if bytes.Compare(dev2, dev) != 0 {
		t.Errorf("wrong device %q != %q", dev2, dev)
	}
	name2 := deviceKeyName(key)
	if bytes.Compare(name2, name) != 0 {
		t.Errorf("wrong name %q != %q", name2, name)
	}
}

func TestGlobalKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")
	name := []byte("name")

	key := globalKey(fld, name)

	fld2 := globalKeyFolder(key)
	if bytes.Compare(fld2, fld) != 0 {
		t.Errorf("wrong folder %q != %q", fld2, fld)
	}
	name2 := globalKeyName(key)
	if bytes.Compare(name2, name) != 0 {
		t.Errorf("wrong name %q != %q", name2, name)
	}
}
