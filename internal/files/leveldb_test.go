// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package files

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
