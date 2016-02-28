// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build darwin

package notify

import (
	"os"
	"testing"
)

func TestCanonicalDarwin(t *testing.T) {
	cases := [...]caseCanonical{
		{"/etc", "/private/etc"},
		{"/etc/defaults", "/private/etc/defaults"},
		{"/etc/hosts", "/private/etc/hosts"},
		{"/tmp", "/private/tmp"},
		{"/var", "/private/var"},
	}
	testCanonical(t, cases[:])
}

func TestCanonicalDarwinMultiple(t *testing.T) {
	etcsym, err := symlink("/etc", "")
	if err != nil {
		t.Fatal(err)
	}
	tmpsym, err := symlink("/tmp", "")
	if err != nil {
		t.Fatal(nonil(err, os.Remove(etcsym)))
	}
	defer removeall(etcsym, tmpsym)
	cases := [...]caseCanonical{
		{etcsym, "/private/etc"},
		{etcsym + "/hosts", "/private/etc/hosts"},
		{tmpsym, "/private/tmp"},
	}
	testCanonical(t, cases[:])
}
