// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/db/sqlite"
	"github.com/syncthing/syncthing/lib/config"
)

var guiCfg config.GUIConfiguration

func init() {
	guiCfg.User = "user"
	guiCfg.SetPassword("pass")
}

func TestStaticAuthOK(t *testing.T) {
	t.Parallel()

	ok := authStatic("user", "pass", guiCfg)
	if !ok {
		t.Fatalf("should pass auth")
	}
}

func TestSimpleAuthUsernameFail(t *testing.T) {
	t.Parallel()

	ok := authStatic("userWRONG", "pass", guiCfg)
	if ok {
		t.Fatalf("should fail auth")
	}
}

func TestStaticAuthPasswordFail(t *testing.T) {
	t.Parallel()

	ok := authStatic("user", "passWRONG", guiCfg)
	if ok {
		t.Fatalf("should fail auth")
	}
}

func TestFormatOptionalPercentS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		template string
		username string
		expected string
	}{
		{"cn=%s,dc=some,dc=example,dc=com", "username", "cn=username,dc=some,dc=example,dc=com"},
		{"cn=fixedusername,dc=some,dc=example,dc=com", "username", "cn=fixedusername,dc=some,dc=example,dc=com"},
		{"cn=%%s,dc=%s,dc=example,dc=com", "username", "cn=%s,dc=username,dc=example,dc=com"},
		{"cn=%%s,dc=%%s,dc=example,dc=com", "username", "cn=%s,dc=%s,dc=example,dc=com"},
		{"cn=%s,dc=%s,dc=example,dc=com", "username", "cn=username,dc=username,dc=example,dc=com"},
	}

	for _, c := range cases {
		templatedDn := formatOptionalPercentS(c.template, c.username)
		if c.expected != templatedDn {
			t.Fatalf("result should be %s != %s", c.expected, templatedDn)
		}
	}
}

func TestEscapeForLDAPFilter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in  string
		out string
	}{
		{"username", `username`},
		{"user(name", `user\28name`},
		{"user)name", `user\29name`},
		{"user\\name", `user\5Cname`},
		{"user*name", `user\2Aname`},
		{"*,CN=asdf", `\2A,CN=asdf`},
	}

	for _, c := range cases {
		res := escapeForLDAPFilter(c.in)
		if c.out != res {
			t.Fatalf("result should be %s != %s", c.out, res)
		}
	}
}

func TestEscapeForLDAPDN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in  string
		out string
	}{
		{"username", `username`},
		{"* ,CN=asdf", `*\20\2CCN\3Dasdf`},
	}

	for _, c := range cases {
		res := escapeForLDAPDN(c.in)
		if c.out != res {
			t.Fatalf("result should be %s != %s", c.out, res)
		}
	}
}

type mockClock struct {
	now time.Time
}

func (c *mockClock) Now() time.Time {
	c.now = c.now.Add(1) // time always ticks by at least 1 ns
	return c.now
}

func (c *mockClock) wind(t time.Duration) {
	c.now = c.now.Add(t)
}

func TestTokenManager(t *testing.T) {
	t.Parallel()

	mdb, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mdb.Close()
	})
	kdb := db.NewMiscDB(mdb)
	clock := &mockClock{now: time.Now()}

	// Token manager keeps up to three tokens with a validity time of 24 hours.
	tm := newTokenManager("testTokens", kdb, 24*time.Hour, 3)
	tm.timeNow = clock.Now

	// Create three tokens
	t0 := tm.New()
	t1 := tm.New()
	t2 := tm.New()

	// Check that the tokens are valid
	if !tm.Check(t0) {
		t.Errorf("token %q should be valid", t0)
	}
	if !tm.Check(t1) {
		t.Errorf("token %q should be valid", t1)
	}
	if !tm.Check(t2) {
		t.Errorf("token %q should be valid", t2)
	}

	// Create a fourth token
	t3 := tm.New()
	// It should be valid
	if !tm.Check(t3) {
		t.Errorf("token %q should be valid", t3)
	}
	// But the first token should have been removed
	if tm.Check(t0) {
		t.Errorf("token %q should be invalid", t0)
	}

	// Wind the clock by 12 hours
	clock.wind(12 * time.Hour)
	// The second token should still be valid (and checking it will give it more life)
	if !tm.Check(t1) {
		t.Errorf("token %q should be valid", t1)
	}

	// Wind the clock by 12 hours
	clock.wind(12 * time.Hour)
	// The second token should still be valid
	if !tm.Check(t1) {
		t.Errorf("token %q should be valid", t1)
	}
	// But the third and fourth tokens should have expired
	if tm.Check(t2) {
		t.Errorf("token %q should be invalid", t2)
	}
	if tm.Check(t3) {
		t.Errorf("token %q should be invalid", t3)
	}
}
