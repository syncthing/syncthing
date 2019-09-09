// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package api

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var passwordHashBytes []byte

func init() {
	passwordHashBytes, _ = bcrypt.GenerateFromPassword([]byte("pass"), 0)
}

func TestStaticAuthOK(t *testing.T) {
	t.Parallel()

	ok := authStatic("user", "pass", "user", string(passwordHashBytes))
	if !ok {
		t.Fatalf("should pass auth")
	}
}

func TestSimpleAuthUsernameFail(t *testing.T) {
	t.Parallel()

	ok := authStatic("userWRONG", "pass", "user", string(passwordHashBytes))
	if ok {
		t.Fatalf("should fail auth")
	}
}

func TestStaticAuthPasswordFail(t *testing.T) {
	t.Parallel()

	ok := authStatic("user", "passWRONG", "user", string(passwordHashBytes))
	if ok {
		t.Fatalf("should fail auth")
	}
}
