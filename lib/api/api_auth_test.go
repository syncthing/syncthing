// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"testing"

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
